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

type ConnectionProfile struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       string `json:"port"`
	User       string `json:"user"`
	Password   string `json:"password"`
	LocalPort  string `json:"localPort"`
	RemotePort string `json:"remotePort"`
}

const (
	LocalAgentAMD64 = "cncyagent_amd64"
	LocalAgentARM64 = "cncyagent_arm64"
	RemotePath      = "/root/cncyagent"
	RemoteLog       = "/root/agent.log"
	HistoryFile     = "history.json"
)

type App struct {
	ctx              context.Context
	sshClient        *ssh.Client
	localListener    net.Listener
	isRunning        bool
	currentLocalPort string
	historyFilePath  string
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
		a.historyFilePath = HistoryFile
	} else {
		a.historyFilePath = filepath.Join(filepath.Dir(exePath), HistoryFile)
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

func (a *App) GetHistory() ([]ConnectionProfile, error) {
	if a.historyFilePath == "" {
		return nil, fmt.Errorf("历史文件路径尚未初始化")
	}

	data, err := os.ReadFile(a.historyFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []ConnectionProfile{}, nil
		}
		return nil, fmt.Errorf("读取历史文件失败: %w", err)
	}

	if len(data) == 0 {
		return []ConnectionProfile{}, nil
	}

	var profiles []ConnectionProfile
	err = json.Unmarshal(data, &profiles)
	if err != nil {
		return nil, fmt.Errorf("解析历史文件JSON失败: %w", err)
	}
	return profiles, nil
}

func (a *App) saveHistory(profile ConnectionProfile) error {
	profiles, _ := a.GetHistory()

	found := false
	for i, p := range profiles {
		if p.Name == profile.Name {
			profiles[i] = profile
			found = true
			break
		}
	}
	if !found {
		profiles = append(profiles, profile)
	}

	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return err
	}

	if a.historyFilePath == "" {
		return fmt.Errorf("无法保存历史记录：文件路径未设置")
	}

	return os.WriteFile(a.historyFilePath, data, 0644)
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
			return fmt.Errorf(errMsg)
		}

		errorString := strings.ToLower(err.Error())
		if strings.Contains(errorString, "permission denied") || strings.Contains(errorString, "unable to authenticate") {
			errMsg := "认证失败：用户名或密码不正确。"
			wailsruntime.MessageDialog(a.ctx, wailsruntime.MessageDialogOptions{
				Type:    wailsruntime.ErrorDialog,
				Title:   "认证失败",
				Message: errMsg,
			})
			return fmt.Errorf(errMsg)
		}

		return fmt.Errorf("未知连接错误: %v", err)
	}
	a.sshClient = client

	a.logUI("🔍 检测架构...")
	sessArch, _ := client.NewSession()
	outArch, err := sessArch.Output("uname -m")
	_ = sessArch.Close()
	if err != nil {
		return fmt.Errorf("架构检测失败: %v", err)
	}

	arch := strings.TrimSpace(string(outArch))
	var localFile string
	if arch == "x86_64" {
		localFile = LocalAgentAMD64
	} else if arch == "aarch64" {
		localFile = LocalAgentARM64
	} else {
		return fmt.Errorf("不支持架构: %s", arch)
	}

	a.logUI("🔍 校验组件...")
	localHash, err := calculateFileHash(localFile)
	if err != nil {
		return fmt.Errorf("计算本地 Agent 哈希失败: %w", err)
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

		if err := a.uploadFile(client, localFile, RemotePath); err != nil {
			return err
		}
	}

	a.logUI("⚙️ 启动服务...")
	startCmd := fmt.Sprintf("setenforce 0 || true; chmod +x %s; nohup %s -port %s > %s 2>&1 < /dev/null &", RemotePath, RemotePath, remotePort, RemoteLog)
	sessStart, _ := client.NewSession()
	err = sessStart.Start(startCmd)
	_ = sessStart.Close()
	if err != nil {
		return fmt.Errorf("启动远程服务失败: %v", err)
	}
	time.Sleep(1 * time.Second)

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

func (a *App) handleTunnelConnection(c net.Conn, remotePort string) {
	defer func() { _ = c.Close() }()
	if a.sshClient == nil {
		return
	}
	rConn, err := a.sshClient.Dial("tcp", "127.0.0.1:"+remotePort)
	if err != nil {
		a.logUI(fmt.Sprintf("无法连接到远端服务: %v", err))
		return
	}
	defer func() { _ = rConn.Close() }()
	go func() { _, _ = io.Copy(rConn, c) }()
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
