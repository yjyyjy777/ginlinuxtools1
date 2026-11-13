package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"golang.org/x/net/context"
)

// [ADDED] Helper function to get top processes
func getTopProcs(sortBy string) []ProcessInfo {
	var procs []ProcessInfo
	// Use ps command to get top 5 processes. The `c` option gives the command name without the full path.
	cmd := exec.Command("bash", "-c", fmt.Sprintf("ps aux --sort=-%s | head -n 6", sortBy))
	out, err := cmd.Output()
	if err != nil {
		return procs
	}

	lines := strings.Split(string(out), "\n")
	// Skip header line
	for i, line := range lines {
		if i == 0 || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 11 {
			// Combine command parts
			command := strings.Join(fields[10:], " ")
			procs = append(procs, ProcessInfo{
				User:    fields[0],
				PID:     fields[1],
				CPU:     fields[2],
				Mem:     fields[3],
				Command: command,
			})
		}
	}
	return procs
}

// [MODIFIED] Helper function to get crontab from /etc/crontab file
func getRootCrontab() []string {
	out, err := os.ReadFile("/etc/crontab")
	if err != nil {
		return []string{"Error reading /etc/crontab: " + err.Error()}
	}
	lines := strings.Split(string(out), "\n")
	var filteredLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			filteredLines = append(filteredLines, trimmed)
		}
	}
	if len(filteredLines) == 0 {
		return []string{"No active (non-commented) cron jobs found in /etc/crontab."}
	}
	return filteredLines
}

// --- 文件上传 ---
func handleUpload(c *gin.Context) {
	// [MODIFIED] 1. 获取文件
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "未接收到文件"})
		return
	}

	uploadType := c.PostForm("upload_type")

	// [MODIFIED] 2. 确定目标目录
	targetDir := c.PostForm("path")
	if uploadType == "update" {
		if targetDir == "" {
			targetDir = "/root/uem-update"
		}
		// 每次上传更新包，都要把已经存在的 /root/uem-update 备份重命名为 /root/uem-update-<目录修改/创建日期>
		cleanTarget := filepath.Clean(targetDir)
		if stat, err := os.Stat(cleanTarget); err == nil && stat.IsDir() {
			modTimeStr := stat.ModTime().Format("20060102_150405")
			backupDir := fmt.Sprintf("%s-%s", cleanTarget, modTimeStr)
			// 如果由于重名等原因存在，增加纳秒避免冲突
			if _, err := os.Stat(backupDir); err == nil {
				backupDir = fmt.Sprintf("%s-%s", cleanTarget, time.Now().Format("20060102_150405"))
			}
			_ = os.Rename(cleanTarget, backupDir)
		}
	} else if uploadType == "install" || uploadType == "pn_install" {
		targetDir = "/root"
	} else {
		isTarGz := strings.HasSuffix(file.Filename, ".tar.gz") || strings.HasSuffix(file.Filename, ".tgz")
		if isTarGz {
			targetDir = "/root"
		} else if targetDir == "" {
			targetDir = "/root"
		}
	}

	// [MODIFIED] 3. 确保目录存在
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		c.JSON(500, gin.H{"error": fmt.Sprintf("无法创建目录 %s: %v", targetDir, err)})
		return
	}

	// [MODIFIED] 4. 保存文件
	dst := filepath.Join(targetDir, file.Filename)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.JSON(500, gin.H{"error": fmt.Sprintf("保存文件失败: %v", err)})
		return
	}

	// [MODIFIED] 5. 解压文件 (针对安装包) 并自动识别解压目录
	isTarGz := strings.HasSuffix(file.Filename, ".tar.gz") || strings.HasSuffix(file.Filename, ".tgz")
	shouldExtract := isTarGz && (uploadType == "install" || uploadType == "pn_install")
	extractedDir := ""
	if shouldExtract {
		cmd := exec.Command("tar", "-zxvf", dst, "-C", targetDir)
		output, err := cmd.CombinedOutput()
		if err != nil {
			c.JSON(500, gin.H{
				"message": "文件上传成功，但安装包解压失败",
				"error":   err.Error(),
				"details": string(output),
			})
			return
		}
		lines := strings.Split(string(output), "\n")
		firstTopDir := ""
		for _, line := range lines {
			cleanLine := strings.TrimPrefix(strings.TrimSpace(line), "./")
			if cleanLine == "" {
				continue
			}
			if firstTopDir == "" {
				seg := strings.Split(cleanLine, "/")[0]
				if seg != "" {
					firstTopDir = filepath.Join(targetDir, seg)
				}
			}
			if cleanLine == "install.sh" || strings.HasSuffix(cleanLine, "/install.sh") {
				relDir := filepath.Dir(cleanLine)
				if relDir == "." {
					extractedDir = targetDir
				} else {
					extractedDir = filepath.Join(targetDir, relDir)
				}
				break
			}
		}
		if extractedDir == "" && firstTopDir != "" {
			if st, err := os.Stat(firstTopDir); err == nil && st.IsDir() {
				extractedDir = firstTopDir
			}
		}
		if uploadType == "pn_install" && extractedDir != "" {
			_ = os.WriteFile("/root/.pn_last_dir", []byte(extractedDir), 0644)
		}
	}

	// [MODIFIED] 6. 返回成功 JSON
	c.JSON(200, gin.H{"message": "上传并处理成功", "path": dst, "dir": targetDir, "extracted_dir": extractedDir})
}

func handleUploadAny(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.String(400, "No file")
		return
	}
	d := c.PostForm("path")
	if d == "" {
		d = UploadTargetDir
	}
	dst := filepath.Join(d, file.Filename)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.String(500, err.Error())
		return
	}
	c.Status(200)
}

// --- 环境检测 ---
func handleCheckEnv(c *gin.Context) {
	res := FullCheckResult{}
	res.SysInfo.CpuCores = runtime.NumCPU()
	res.SysInfo.CpuPass = res.SysInfo.CpuCores >= 2
	mkb := getMemTotalKB()
	res.SysInfo.MemTotal = fmt.Sprintf("%.1f GB", float64(mkb)/1024/1024)
	res.SysInfo.MemPass = float64(mkb)/1024/1024 >= 7.5
	res.SysInfo.Arch = runtime.GOARCH
	res.SysInfo.OsName = getOSName()
	lo := strings.ToLower(res.SysInfo.OsName)
	res.SysInfo.OsPass = (strings.Contains(lo, "kylin") && strings.Contains(lo, "v10")) || (strings.Contains(lo, "rocky") && strings.Contains(lo, "9"))
	if mkb > 0 {
		avail := getMemAvailableKB()
		if avail > 0 {
			res.SysInfo.MemUsage = float64(mkb-avail) / float64(mkb) * 100
		}
	}
	res.SysInfo.LoadAvg = getLoadAvg()
	rx, tx := getNetIO()
	res.SysInfo.NetRx = rx
	res.SysInfo.NetTx = tx
	out, _ := exec.Command("bash", "-c", "ulimit -n").Output()
	res.SysInfo.Ulimit = strings.TrimSpace(string(out))
	res.SysInfo.UlimitPass = res.SysInfo.Ulimit != "1024"
	cmd := exec.Command("df", "-h")
	out, _ = cmd.Output()
	res.SysInfo.DiskDetail = string(out)
	for i, line := range strings.Split(string(out), "\n") {
		if i == 0 || len(line) == 0 {
			continue
		}
		f := strings.Fields(line)
		if len(f) >= 6 && !strings.Contains(f[0], "tmp") && !strings.Contains(f[0], "over") {
			u, _ := strconv.Atoi(strings.TrimRight(f[4], "%"))
			res.SysInfo.DiskList = append(res.SysInfo.DiskList, DiskInfo{Mount: f[5], Total: f[1], Used: f[2], Usage: u})
		}
	}
	// Network info
	netstatOut, _ := exec.Command("netstat", "-nltp").Output()
	var netstatList []NetstatInfo
	for _, line := range strings.Split(string(netstatOut), "\n") {
		if !strings.HasPrefix(line, "tcp") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 7 {
			netstatList = append(netstatList, NetstatInfo{
				Proto:   fields[0],
				Address: fields[3],
				Pid:     fields[6],
			})
		}
	}
	res.SysInfo.Netstat = netstatList

	tcpCountOut, _ := exec.Command("bash", "-c", "netstat -an | grep -ic TCP").Output()
	tcpCount, _ := strconv.Atoi(strings.TrimSpace(string(tcpCountOut)))
	res.SysInfo.TcpConnCount = tcpCount

	// [ADDED] Get Top Processes and Crontab
	res.SysInfo.TopCPUProcs = getTopProcs("cpu")
	res.SysInfo.TopMemProcs = getTopProcs("pmem") // pmem or rss, depending on desired memory metric
	res.SysInfo.RootCrontab = getRootCrontab()

	if o, err := exec.Command("getenforce").Output(); err == nil {
		res.SecInfo.SELinux = strings.TrimSpace(string(o))
	} else {
		res.SecInfo.SELinux = "?"
	}
	fw := "Stopped"
	if err := exec.Command("systemctl", "is-active", "firewalld").Run(); err == nil {
		fw = "Running"
	}
	res.SecInfo.Firewall = fw
	res.SecInfo.SshTunnelOk = checkSshConfig()

	// --- UEM / PN 服务状态检查 (基于 cncymdm / cncypn) ---
	cncymdmPath := findCmdScript("cncymdm")
	cncypnPath := findCmdScript("cncypn")

	var targetServices []string
	if cncymdmPath != "" {
		res.UemInfo.Installed = true
		res.UemInfo.ServerType = "uem"
		targetServices = parseScriptServices(cncymdmPath, []string{"mdm", "base"}, uemServices)
	} else if cncypnPath != "" {
		res.UemInfo.Installed = true
		res.UemInfo.ServerType = "pn"
		targetServices = parseScriptServices(cncypnPath, []string{"pn_services", "base_services"}, pnServices)
	} else {
		res.UemInfo.Installed = false
		res.UemInfo.ServerType = "none"
	}

	if res.UemInfo.Installed {
		for _, s := range targetServices {
			st := "stop"
			if err := exec.Command("systemctl", "is-active", "--quiet", s).Run(); err == nil {
				st = "run"
			}

			displayName := s
			if s == "rabbitmq-server" {
				displayName = "rabbitmq"
			}

			res.UemInfo.Services = append(res.UemInfo.Services, ServiceStat{Name: displayName, Status: st})
		}
	}
	// --- 检查结束 ---

	// MinIO 检测: PN 服务器和没有 minio 服务的系统不检测/不显示
	if hasMinioService(res.UemInfo.ServerType) {
		res.MinioInfo.HasService = true
		mClient, err := minio.New(MinioEndpoint, &minio.Options{Creds: credentials.NewStaticV4(MinioUser, MinioPass, ""), Secure: false})
		if err == nil {
			exists, _ := mClient.BucketExists(context.Background(), MinioBucket)
			if exists {
				res.MinioInfo.BucketExists = true
				p, _ := mClient.GetBucketPolicy(context.Background(), MinioBucket)
				if strings.Contains(p, "GetObject") && strings.Contains(p, "*") {
					res.MinioInfo.Policy = "public"
				} else {
					res.MinioInfo.Policy = "private"
				}
			}
		}
	}

	c.JSON(200, res)
}

func findCmdScript(name string) string {
	candidates := []string{
		"/usr/bin/" + name,
		"/usr/local/bin/" + name,
		"/bin/" + name,
		"/sbin/" + name,
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

func parseScriptServices(scriptPath string, varNames []string, fallback []string) []string {
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return fallback
	}
	lines := strings.Split(string(data), "\n")
	var result []string
	seen := make(map[string]bool)
	for _, vName := range varNames {
		prefix := vName + "="
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, prefix) {
				val := strings.TrimPrefix(trimmed, prefix)
				val = strings.Trim(val, `"'`)
				for _, svc := range strings.Fields(val) {
					if svc != "" && !seen[svc] {
						seen[svc] = true
						result = append(result, svc)
					}
				}
				break
			}
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}

func hasMinioService(serverType string) bool {
	if serverType == "pn" {
		return false
	}
	if _, err := os.Stat("/etc/systemd/system/minio.service"); err == nil {
		return true
	}
	if _, err := os.Stat("/usr/lib/systemd/system/minio.service"); err == nil {
		return true
	}
	out, err := exec.Command("systemctl", "show", "-p", "LoadState", "--value", "minio").Output()
	if err == nil && strings.TrimSpace(string(out)) == "loaded" {
		return true
	}
	return false
}

func handleRestartService(c *gin.Context) {
	name := c.Query("name")
	if name == "rabbitmq" {
		name = "rabbitmq-server"
	}
	cmd := exec.Command("systemctl", "restart", name)
	err := cmd.Run()
	if err != nil {
		log.Printf("[ServiceRestart] Failed to restart service '%s': %v", name, err)
	}
	c.String(200, "Done")
}

func handleFixMinio(c *gin.Context) {
	mClient, err := minio.New(MinioEndpoint, &minio.Options{Creds: credentials.NewStaticV4(MinioUser, MinioPass, ""), Secure: false})
	if err != nil {
		log.Printf("[MinioFix] Failed to create minio client: %v", err)
		c.String(500, "Failed to create minio client")
		return
	}
	p := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetBucketLocation","s3:ListBucket"],"Resource":["arn:aws:s3:::%s"]},{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::%s/*"]}]}`, MinioBucket, MinioBucket)
	err = mClient.SetBucketPolicy(context.Background(), MinioBucket, p)
	if err != nil {
		log.Printf("[MinioFix] Failed to set bucket policy: %v", err)
		c.String(500, "Failed to set bucket policy")
		return
	}
	c.String(200, "Done")
}

func handleFixSsh(c *gin.Context) {
	_ = autoFixSshConfig()
	c.String(200, "Done")
}

func handleFixSelinux(c *gin.Context) {
	_ = exec.Command("setenforce", "0").Run()
	d, _ := os.ReadFile("/etc/selinux/config")
	_ = os.WriteFile("/etc/selinux/config", []byte(strings.Replace(string(d), "SELINUX=enforcing", "SELINUX=disabled", 1)), 0644)
	c.String(200, "Done")
}

func handleFixFirewall(c *gin.Context) {
	_ = exec.Command("systemctl", "stop", "firewalld").Run()
	_ = exec.Command("systemctl", "disable", "firewalld").Run()
	c.String(200, "Done")
}

// --- 文件系统操作 ---
func handleFsList(c *gin.Context) {
	dir := c.Query("path")
	if dir == "" {
		dir = "/root"
	}
	es, _ := os.ReadDir(dir)
	var fs []FileInfo
	for _, e := range es {
		i, _ := e.Info()
		sz := "-"
		if !e.IsDir() {
			sz = formatBytes(i.Size())
		}
		fs = append(fs, FileInfo{Name: e.Name(), Path: filepath.Join(dir, e.Name()), IsDir: e.IsDir(), Size: sz, ModTime: i.ModTime().Format("2006-01-02 15:04")})
	}
	c.JSON(200, fs)
}

func handleFsDownload(c *gin.Context) {
	path := c.Query("path")
	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil || info.IsDir() {
		c.Status(404)
		return
	}
	fileName := filepath.Base(cleanPath)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	c.File(cleanPath)
}

// --- 日志 ---
func handleLogDownload(c *gin.Context) {
	key := c.Query("key")
	path, ok := logFileMap[key]
	if !ok {
		c.Status(404)
		return
	}
	_ = os.Chmod(path, 0644)
	c.File(path)
}
