package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"
)

//go:embed all:frontend
var assets embed.FS

//go:embed all:pkg
var embeddedPkg embed.FS

type ConnectionProfile struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       string `json:"port"`
	User       string `json:"user"`
	Password   string `json:"password"`
	LocalPort  string `json:"localPort"`
	RemotePort string `json:"remotePort"`
}

type HistoryData struct {
	Profiles []ConnectionProfile `json:"profiles"`
	Buckets  []string            `json:"buckets"`
}

const (
	LocalAgentAMD64   = "cncyagent_amd64"
	LocalAgentARM64   = "cncyagent_arm64"
	RemotePath        = "/root/cncyagent"
	RemoteLog         = "/root/agent.log"
	HistoryFile       = "history.json"
	RemoteHistoryPath = "/root/history.json"
	DefaultBucketURL  = "http://f.cncytech.com:9000/cymdm62/"
)

type App struct {
	ctx              context.Context
	sshClient        *ssh.Client
	localListener    net.Listener
	isRunning        bool
	currentLocalPort string
	historyFilePath  string
	exeDir           string // [ADDED] Directory of the executable
}

// ProgressWriter is a custom writer that reports progress
type ProgressWriter struct {
	writer     io.Writer
	total      int64
	written    int64
	onProgress func(float64)
}

func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	if err == nil {
		pw.written += int64(n)
		progress := float64(pw.written) / float64(pw.total) * 100
		pw.onProgress(progress)
	}
	return n, err
}

func NewApp() *App {
	return &App{}
}

// startup 应用启动时调用
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 获取可执行文件所在的目录
	exePath, err := os.Executable()
	if err != nil {
		// Fallback for safety, though it should rarely fail
		a.exeDir = "."
		a.historyFilePath = HistoryFile
	} else {
		a.exeDir = filepath.Dir(exePath) // [MODIFIED] Store the executable's directory
		a.historyFilePath = filepath.Join(a.exeDir, HistoryFile)
	}

	// 启动一个 goroutine，在短暂延迟后加载历史记录并推送到前端
	go func() {
		time.Sleep(500 * time.Millisecond)
		history, err := a.GetHistory()
		if err != nil {
			wailsruntime.EventsEmit(a.ctx, "history:loaded", []ConnectionProfile{})
			return
		}
		wailsruntime.EventsEmit(a.ctx, "history:loaded", history)
	}()
}

func (a *App) shutdown(ctx context.Context) {
	a.StopDeploy()
}

func parseHistoryBytes(data []byte) (HistoryData, error) {
	defaultData := HistoryData{
		Profiles: []ConnectionProfile{},
		Buckets:  []string{DefaultBucketURL},
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return defaultData, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var profiles []ConnectionProfile
		if err := json.Unmarshal(data, &profiles); err != nil {
			return defaultData, err
		}
		defaultData.Profiles = profiles
		return defaultData, nil
	}
	var hd HistoryData
	if err := json.Unmarshal(data, &hd); err != nil {
		return defaultData, err
	}
	if hd.Profiles == nil {
		hd.Profiles = []ConnectionProfile{}
	}
	if len(hd.Buckets) == 0 {
		hd.Buckets = []string{DefaultBucketURL}
	}
	return hd, nil
}

func (a *App) loadHistoryData() (HistoryData, error) {
	if a.historyFilePath == "" {
		return HistoryData{Profiles: []ConnectionProfile{}, Buckets: []string{DefaultBucketURL}}, fmt.Errorf("历史文件路径尚未初始化")
	}

	data, err := os.ReadFile(a.historyFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return HistoryData{Profiles: []ConnectionProfile{}, Buckets: []string{DefaultBucketURL}}, nil
		}
		return HistoryData{}, fmt.Errorf("读取历史文件失败: %w", err)
	}

	return parseHistoryBytes(data)
}

func (a *App) saveHistoryData(hd HistoryData) error {
	if a.historyFilePath == "" {
		return fmt.Errorf("无法保存历史记录：文件路径未设置")
	}
	if hd.Profiles == nil {
		hd.Profiles = []ConnectionProfile{}
	}
	if len(hd.Buckets) == 0 {
		hd.Buckets = []string{DefaultBucketURL}
	}
	data, err := json.MarshalIndent(hd, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.historyFilePath, data, 0644)
}

func (a *App) GetHistory() ([]ConnectionProfile, error) {
	hd, err := a.loadHistoryData()
	if err != nil {
		return nil, err
	}
	return hd.Profiles, nil
}

func (a *App) saveHistory(profile ConnectionProfile) error {
	hd, _ := a.loadHistoryData()

	found := false
	for i, p := range hd.Profiles {
		if p.Name == profile.Name {
			hd.Profiles[i] = profile
			found = true
			break
		}
	}
	if !found {
		hd.Profiles = append(hd.Profiles, profile)
	}

	if err := a.saveHistoryData(hd); err != nil {
		return err
	}
	if a.sshClient != nil {
		a.syncHistoryToRemote(a.sshClient)
	}
	return nil
}

func (a *App) syncHistoryToRemote(client *ssh.Client) {
	if client == nil {
		return
	}
	localHD, _ := a.loadHistoryData()
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return
	}
	defer func() { _ = sftpClient.Close() }()

	// 如果远端已有 /root/history.json，合并其 buckets 防止丢失
	if rf, err := sftpClient.Open(RemoteHistoryPath); err == nil {
		if rBytes, err := io.ReadAll(rf); err == nil {
			if remoteHD, err := parseHistoryBytes(rBytes); err == nil {
				seen := make(map[string]bool)
				for _, b := range localHD.Buckets {
					seen[b] = true
				}
				for _, b := range remoteHD.Buckets {
					if b != "" && !seen[b] {
						localHD.Buckets = append(localHD.Buckets, b)
						seen[b] = true
					}
				}
			}
		}
		_ = rf.Close()
	}

	_ = a.saveHistoryData(localHD)
	if data, err := json.MarshalIndent(localHD, "", "  "); err == nil {
		if wf, err := sftpClient.Create(RemoteHistoryPath); err == nil {
			_, _ = wf.Write(data)
			_ = wf.Close()
		}
	}
}

func (a *App) pullHistoryFromRemote() {
	if a.sshClient == nil {
		return
	}
	sftpClient, err := sftp.NewClient(a.sshClient)
	if err != nil {
		return
	}
	defer func() { _ = sftpClient.Close() }()

	rf, err := sftpClient.Open(RemoteHistoryPath)
	if err != nil {
		return
	}
	defer func() { _ = rf.Close() }()

	rBytes, err := io.ReadAll(rf)
	if err != nil {
		return
	}
	remoteHD, err := parseHistoryBytes(rBytes)
	if err != nil {
		return
	}

	localHD, _ := a.loadHistoryData()
	localHD.Buckets = remoteHD.Buckets
	_ = a.saveHistoryData(localHD)
}

func (a *App) StartDeploy(host, port, user, pass, localPort, remotePort string) error {
	if a.isRunning {
		return fmt.Errorf("一个任务正在运行中")
	}

	a.logUI("🚀 正在连接服务器...")

	// 直接执行部署流程，不再放到 goroutine 中
	err := a.runDeployProcess(host, port, user, pass, localPort, remotePort)
	if err != nil {
		// 如果连接或部署失败，立即将错误返回给前端
		a.logUI(fmt.Sprintf("❌ 开始失败: %s", err.Error()))
		a.updateStatus(false, "", "")
		return err
	}

	// 只有在成功后才更新状态和 UI
	a.isRunning = true
	a.currentLocalPort = localPort
	a.logUI(fmt.Sprintf("✅ 运行中 | 本地: %s <-> 远端: %s", localPort, remotePort))
	a.updateStatus(true, localPort, remotePort)

	// 构造当前连接配置
	profile := ConnectionProfile{
		Name:       fmt.Sprintf("%s@%s", user, host),
		Host:       host,
		Port:       port,
		User:       user,
		Password:   pass,
		LocalPort:  localPort,
		RemotePort: remotePort,
	}

	// 在后台保存历史记录和打开浏览器，不阻塞主流程
	go func() {
		if err := a.saveHistory(profile); err != nil {
			a.logUI("⚠️ 警告: 保存历史记录失败")
		} else {
			if history, err := a.GetHistory(); err == nil {
				wailsruntime.EventsEmit(a.ctx, "history:loaded", history)
			}
		}
		a.OpenBrowser()
	}()

	return nil
}

func (a *App) StopDeploy() {
	if !a.isRunning {
		return
	}
	a.logUI("👋 正在停止服务...")
	if a.sshClient != nil {
		a.pullHistoryFromRemote()
		s, err := a.sshClient.NewSession()
		if err == nil {
			_ = s.Run("pkill -f cncyagent")
			_ = s.Close()
		}
		_ = a.sshClient.Close()
		a.sshClient = nil
	}
	if a.localListener != nil {
		_ = a.localListener.Close()
		a.localListener = nil
	}
	a.isRunning = false
	a.currentLocalPort = ""
	a.logUI("服务已停止")
	a.updateStatus(false, "", "")
}

func (a *App) OpenBrowser() {
	if a.currentLocalPort == "" {
		return
	}
	url := "http://localhost:" + a.currentLocalPort
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default:
		cmd = "xdg-open"
	}
	args = append(args, url)
	if err := exec.Command(cmd, args...).Start(); err != nil {
		a.logUI(fmt.Sprintf("无法打开浏览器: %v", err))
	}
}

func calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (a *App) runDeployProcess(host, port, user, pass, localPort, remotePort string) error {
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%s", host, port), config)
	if err != nil {
		// --- 核心错误判断逻辑 ---
		var netErr net.Error
		if errors.As(err, &netErr) {
			var errMsg string
			if netErr.Timeout() {
				errMsg = "网络连接超时：无法在规定时间内连接到服务器。请检查网络或防火墙设置。"
			} else {
				errMsg = "网络错误：无法连接到服务器。请检查主机地址、端口和网络连通性。"
			}
			wailsruntime.MessageDialog(a.ctx, wailsruntime.MessageDialogOptions{
				Type:    wailsruntime.ErrorDialog,
				Title:   "网络错误",
				Message: errMsg,
			})
			return errors.New(errMsg)
		}

		errorString := strings.ToLower(err.Error())
		if strings.Contains(errorString, "permission denied") || strings.Contains(errorString, "unable to authenticate") {
			errMsg := "认证失败：用户名或密码不正确。"
			wailsruntime.MessageDialog(a.ctx, wailsruntime.MessageDialogOptions{
				Type:    wailsruntime.ErrorDialog,
				Title:   "认证失败",
				Message: errMsg,
			})
			return errors.New(errMsg)
		}

		return fmt.Errorf("未知连接错误: %v", err)
	}
	a.sshClient = client

	a.logUI("🔍 检测系统架构与环境命令...")
	sessArch, _ := client.NewSession()
	outArch, err := sessArch.Output("uname -m")
	_ = sessArch.Close()
	if err != nil {
		return fmt.Errorf("架构检测失败: %v", err)
	}

	arch := strings.TrimSpace(string(outArch))
	var localAgentFilename string
	if arch == "x86_64" {
		localAgentFilename = LocalAgentAMD64
	} else if arch == "aarch64" {
		localAgentFilename = LocalAgentARM64
	} else {
		return fmt.Errorf("不支持架构: %s", arch)
	}

	// 检查远端是否缺失 tar 或 wget
	a.ensureBaseCommands(client, arch)

	// [MODIFIED] Construct absolute path for the agent file
	localAgentPath := filepath.Join(a.exeDir, localAgentFilename)

	// [FIX] Mac App Bundle Path Resolution
	if runtime.GOOS == "darwin" {
		if _, err := os.Stat(localAgentPath); os.IsNotExist(err) {
			// If running inside .app/Contents/MacOS, the sibling files are 3 levels up
			// .app/Contents/MacOS/../../../cncyagent_amd64
			upperPath := filepath.Join(a.exeDir, "../../..", localAgentFilename)
			if _, err := os.Stat(upperPath); err == nil {
				localAgentPath = upperPath
			}
		}
	}

	a.logUI("🔍 校验组件...")
	localHash, err := calculateFileHash(localAgentPath) // [MODIFIED] Use absolute path
	if err != nil {
		// [MODIFIED] Provide a more user-friendly error message
		errorMsg := fmt.Sprintf("无法读取本地 Agent 文件 '%s'。请确保它与应用程序在同一目录中。", localAgentFilename)
		a.logUI("❌ " + errorMsg)
		wailsruntime.MessageDialog(a.ctx, wailsruntime.MessageDialogOptions{
			Type:    wailsruntime.ErrorDialog,
			Title:   "文件未找到",
			Message: errorMsg,
		})
		return fmt.Errorf("%s: %w", errorMsg, err)
	}

	remoteHashCmd := fmt.Sprintf("sha256sum %s", RemotePath)
	remoteSess, _ := client.NewSession()
	remoteOut, err := remoteSess.CombinedOutput(remoteHashCmd)
	_ = remoteSess.Close()

	uploadNeeded := true
	if err == nil {
		remoteHash := strings.Fields(string(remoteOut))[0]
		if remoteHash == localHash {
			uploadNeeded = false
		}
	}

	if uploadNeeded {
		a.logUI("📤 上传组件...")
		sessClean, _ := client.NewSession()
		_ = sessClean.Run(fmt.Sprintf("pkill -f cncyagent; rm -f %s", RemotePath))
		_ = sessClean.Close()
		time.Sleep(500 * time.Millisecond)

		if err := a.uploadFile(client, localAgentPath, RemotePath); err != nil { // [MODIFIED] Use absolute path
			return err
		}
	}

	// 同步本地与远端 history.json (包含桶配置)
	a.syncHistoryToRemote(client)

	a.logUI("⚙️ 启动服务...")
	startCmd := fmt.Sprintf("setenforce 0 || true; chmod +x %s; nohup %s -port %s > %s 2>&1 < /dev/null &", RemotePath, RemotePath, remotePort, RemoteLog)
	sessStart, _ := client.NewSession()
	err = sessStart.Start(startCmd)
	time.Sleep(500 * time.Millisecond)
	_ = sessStart.Close()
	if err != nil {
		return fmt.Errorf("启动远程服务失败: %v", err)
	}

	// 主动轮询检测远端 Agent 端口是否就绪（最多等待 5 秒）
	for i := 0; i < 20; i++ {
		testConn, dialErr := client.Dial("tcp", "127.0.0.1:"+remotePort)
		if dialErr == nil {
			_ = testConn.Close()
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	a.logUI(fmt.Sprintf("🔗 建立隧道 %s -> %s...", localPort, remotePort))
	listener, err := net.Listen("tcp", "localhost:"+localPort)
	if err != nil {
		return fmt.Errorf("本地端口占用")
	}
	a.localListener = listener

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				if a.isRunning {
					a.logUI(fmt.Sprintf("隧道监听失败: %v", err))
				}
				return
			}
			go a.handleTunnelConnection(conn, remotePort)
		}
	}()
	return nil
}

type bucketSyncInspectWriter struct {
	w      io.Writer
	app    *App
	synced bool
}

func (bw *bucketSyncInspectWriter) Write(p []byte) (int, error) {
	if !bw.synced && (strings.Contains(string(p), "/api/buckets") || strings.Contains(string(p), "api/buckets")) {
		bw.synced = true
		go func() {
			time.Sleep(400 * time.Millisecond)
			bw.app.pullHistoryFromRemote()
		}()
	}
	return bw.w.Write(p)
}

func (a *App) handleTunnelConnection(c net.Conn, remotePort string) {
	defer func() { _ = c.Close() }()
	if a.sshClient == nil {
		return
	}
	var rConn net.Conn
	var err error
	for attempt := 0; attempt < 6; attempt++ {
		if a.sshClient == nil {
			return
		}
		rConn, err = a.sshClient.Dial("tcp", "127.0.0.1:"+remotePort)
		if err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		a.logUI(fmt.Sprintf("无法连接到远端服务: %v", err))
		return
	}
	defer func() { _ = rConn.Close() }()
	go func() {
		_, _ = io.Copy(&bucketSyncInspectWriter{w: rConn, app: a}, c)
	}()
	_, _ = io.Copy(c, rConn)
}

func (a *App) uploadFile(client *ssh.Client, local, remote string) error {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer func() { _ = sftpClient.Close() }()

	src, err := os.Open(local)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	fileInfo, err := src.Stat()
	if err != nil {
		return err
	}
	fileSize := fileInfo.Size()

	dst, err := sftpClient.Create(remote)
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	progressWriter := &ProgressWriter{
		writer: dst,
		total:  fileSize,
		onProgress: func(progress float64) {
			wailsruntime.EventsEmit(a.ctx, "upload:progress", progress)
		},
	}

	_, err = io.Copy(progressWriter, src)
	return err
}

// 检查并自动安装缺失的 tar 和 wget 命令
func (a *App) ensureBaseCommands(client *ssh.Client, arch string) {
	a.logUI("🔍 正在检查服务器基础命令 (tar / wget)...")
	time.Sleep(500 * time.Millisecond)

	// 1. 分别测试命令是否真正能正常执行运行 (运行 --version 确保动态链接库不报错)
	checkCmd := func(cmd string) bool {
		sess, err := client.NewSession()
		if err != nil {
			return false
		}
		defer sess.Close()
		testCmd := fmt.Sprintf("%s --version >/dev/null 2>&1", cmd)
		err = sess.Run(testCmd)
		return err == nil
	}

	hasTar := checkCmd("tar")
	hasWget := checkCmd("wget")

	if hasTar && hasWget {
		a.logUI("✅ 服务器基础命令 (tar / wget) 均已就绪")
		time.Sleep(600 * time.Millisecond)
		return
	}

	var missing []string
	if !hasTar {
		missing = append(missing, "tar")
	}
	if !hasWget {
		missing = append(missing, "wget")
	}
	a.logUI(fmt.Sprintf("⚠️ 检测到服务器缺失命令: %s", strings.Join(missing, ", ")))
	time.Sleep(1 * time.Second)

	// 2. 检测系统发行版与架构，严格匹配 r98 / ky86 / kyarm
	relSess, err := client.NewSession()
	var osInfo string
	if err == nil {
		relOut, _ := relSess.CombinedOutput("cat /etc/os-release 2>/dev/null || cat /etc/redhat-release 2>/dev/null || cat /etc/issue 2>/dev/null || uname -a")
		_ = relSess.Close()
		osInfo = strings.ToLower(string(relOut))
	}

	// 决策目录：
	// 麒麟系统 (kylin / neokylin) -> aarch64 用 kyarm，x86_64 用 ky86
	// Rocky / RHEL / CentOS / Alma / 其他红帽系 -> 用 r98
	pkgSubDir := ""
	isKylin := strings.Contains(osInfo, "kylin") || strings.Contains(osInfo, "neokylin")
	if isKylin {
		if arch == "aarch64" {
			pkgSubDir = "kyarm"
		} else {
			pkgSubDir = "ky86"
		}
	} else {
		// 严密防误判：非麒麟系统根据架构，x86 强制匹配 r98，aarch64 如是麒麟以外暂归 kyarm
		if arch == "x86_64" {
			pkgSubDir = "r98"
		} else if arch == "aarch64" {
			pkgSubDir = "kyarm"
		} else {
			pkgSubDir = "r98"
		}
	}
	a.logUI(fmt.Sprintf("📌 识别系统分支: %s / %s -> 对应依赖目录: pkg/%s", arch, func() string {
		if isKylin {
			return "银河麒麟"
		}
		return "Rocky/RHEL9"
	}(), pkgSubDir))
	time.Sleep(1000 * time.Millisecond)

	// 3. 读取待安装的 RPM 资源 (支持内嵌资源 + 磁盘外部目录)
	type rpmEntry struct {
		name string
		open func() (io.ReadCloser, error)
	}

	var availableRpms []rpmEntry

	// A. 尝试从本地磁盘目录加载
	cwd, _ := os.Getwd()
	candidates := []string{
		filepath.Join(a.exeDir, "pkg", pkgSubDir),
		filepath.Join(cwd, "pkg", pkgSubDir),
		filepath.Join(a.exeDir, "..", "pkg", pkgSubDir),
		filepath.Join("dist", "windows", "uemtools", "pkg", pkgSubDir),
		filepath.Join("dist", "windows", "pkg", pkgSubDir),
		filepath.Join("pkg", pkgSubDir),
	}

	for _, c := range candidates {
		if stat, err := os.Stat(c); err == nil && stat.IsDir() {
			if entries, err := os.ReadDir(c); err == nil {
				for _, entry := range entries {
					if strings.HasSuffix(entry.Name(), ".rpm") {
						p := filepath.Join(c, entry.Name())
						availableRpms = append(availableRpms, rpmEntry{
							name: entry.Name(),
							open: func() (io.ReadCloser, error) { return os.Open(p) },
						})
					}
				}
				if len(availableRpms) > 0 {
					a.logUI(fmt.Sprintf("📂 从外部目录 %s 加载依赖包", c))
					time.Sleep(500 * time.Millisecond)
					break
				}
			}
		}
	}

	// B. 如果外部目录没有找到，则自动从程序内嵌资源中加载 (保证免放目录也能生效)
	if len(availableRpms) == 0 {
		embedPath := "pkg/" + pkgSubDir
		if entries, err := embeddedPkg.ReadDir(embedPath); err == nil {
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".rpm") {
					name := entry.Name()
					fullP := embedPath + "/" + name
					availableRpms = append(availableRpms, rpmEntry{
						name: name,
						open: func() (io.ReadCloser, error) { return embeddedPkg.Open(fullP) },
					})
				}
			}
			if len(availableRpms) > 0 {
				a.logUI("📦 从客户端内置资源中加载依赖包")
				time.Sleep(500 * time.Millisecond)
			}
		}
	}

	if len(availableRpms) == 0 {
		a.logUI(fmt.Sprintf("⚠️ 未在本地或内嵌资源中找到 pkg/%s 的 RPM 包，跳过自动补全", pkgSubDir))
		time.Sleep(1500 * time.Millisecond)
		return
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		a.logUI(fmt.Sprintf("⚠️ 建立 SFTP 失败: %v", err))
		time.Sleep(1000 * time.Millisecond)
		return
	}
	defer sftpClient.Close()

	_ = sftpClient.MkdirAll("/tmp/pkg_install")

	var rpmsToInstall []string
	for _, item := range availableRpms {
		name := item.name
		isTarPkg := strings.HasPrefix(name, "tar-") || strings.HasPrefix(name, "tar_")
		isWgetPkg := strings.HasPrefix(name, "wget-") || strings.HasPrefix(name, "wget_")

		if (!hasTar && isTarPkg) || (!hasWget && isWgetPkg) {
			remotePath := "/tmp/pkg_install/" + name
			a.logUI(fmt.Sprintf("📤 正在上传 %s...", name))

			sf, err := item.open()
			if err == nil {
				df, err := sftpClient.Create(remotePath)
				if err == nil {
					_, _ = io.Copy(df, sf)
					df.Close()
					rpmsToInstall = append(rpmsToInstall, remotePath)
				} else {
					a.logUI(fmt.Sprintf("❌ 远端创建文件失败: %v", err))
				}
				sf.Close()
			} else {
				a.logUI(fmt.Sprintf("❌ 打开依赖包失败: %v", err))
			}
		}
	}

	if len(rpmsToInstall) > 0 {
		a.logUI("🛠️ 正在执行 rpm 安装命令...")
		instSess, err := client.NewSession()
		if err == nil {
			cmdStr := fmt.Sprintf("rpm -Uvh --replacepkgs --nodeps %s; rm -rf /tmp/pkg_install", strings.Join(rpmsToInstall, " "))
			instOut, err := instSess.CombinedOutput(cmdStr)
			_ = instSess.Close()
			if err != nil {
				a.logUI(fmt.Sprintf("⚠️ rpm 安装命令输出: %s (错误: %v)", string(instOut), err))
			} else {
				a.logUI("✅ 成功安装缺失命令！")
			}
		}
		time.Sleep(1500 * time.Millisecond)
	} else {
		a.logUI("⚠️ 未匹配到缺失命令对应的具体 RPM 包")
		time.Sleep(1500 * time.Millisecond)
	}
}

func (a *App) logUI(message string) {
	wailsruntime.EventsEmit(a.ctx, "log:update", message)
}

func (a *App) updateStatus(running bool, localPort, remotePort string) {
	payload := map[string]interface{}{
		"isRunning":  running,
		"localPort":  localPort,
		"remotePort": remotePort,
	}
	wailsruntime.EventsEmit(a.ctx, "status:update", payload)
}

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:     "UEM Deployment Tools",
		Width:     500,
		Height:    480, // [MODIFIED] 默认高度与最小高度一致
		MinWidth:  450,
		MinHeight: 480, // [MODIFIED] 保持最小高度为 480
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatal(err)
	}
}
