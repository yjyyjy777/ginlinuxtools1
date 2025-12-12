package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"golang.org/x/net/context"
)

// --- 文件上传 ---
func handleUpload(c *gin.Context) {
	// [MODIFIED] 1. 获取用户指定的路径，默认为 /root
	targetDir := c.PostForm("path")
	if targetDir == "" {
		targetDir = "/root"
	}

	// [MODIFIED] 2. 获取文件
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "未接收到文件"})
		return
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

	// [MODIFIED] 5. 解压文件 (仅针对 .tar.gz)
	if strings.HasSuffix(file.Filename, ".tar.gz") || strings.HasSuffix(file.Filename, ".tgz") {
		cmd := exec.Command("tar", "-zxvf", dst, "-C", targetDir)
		output, err := cmd.CombinedOutput()
		if err != nil {
			// 即使解压失败，也返回详细信息
			c.JSON(500, gin.H{
				"message": "文件上传成功，但解压失败",
				"error":   err.Error(),
				"details": string(output),
			})
			return
		}
	}

	// [MODIFIED] 6. 返回成功 JSON
	c.JSON(200, gin.H{"message": "上传并处理成功", "path": dst})
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

	// --- UEM 服务状态检查 (最终正确版) ---
	if _, err := os.Stat("/opt/emm/current"); err == nil {
		res.UemInfo.Installed = true
		for _, s := range uemServices {
			st := "stop"
			cmd := exec.Command("systemctl", "show", "-p", "ActiveState", "--value", s)
			output, err := cmd.CombinedOutput()

			if err == nil && strings.Contains(string(output), "active") {
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
	c.JSON(200, res)
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
	c.File(path)
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

// --- 首页 ---
func handleIndex(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, htmlPage)
}
