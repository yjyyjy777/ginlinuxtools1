package main

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	DefaultRemoteBucket = "http://f.cncytech.com:9000/cymdm62/"
	AgentHistoryPath    = "/root/history.json"
)

type S3ListBucketResult struct {
	XMLName  xml.Name    `xml:"ListBucketResult"`
	Name     string      `xml:"Name"`
	Contents []S3Content `xml:"Contents"`
}

type S3Content struct {
	Key          string `xml:"Key" json:"key"`
	LastModified string `xml:"LastModified" json:"last_modified"`
	Size         int64  `xml:"Size" json:"size"`
}

type agentHistoryFile struct {
	Profiles json.RawMessage `json:"profiles"`
	Buckets  []string        `json:"buckets"`
}

// 解析用户输入的桶地址，兼容：
// 1. S3 直连格式: http://f.cncytech.com:9000/cymdm62/
// 2. MinIO 控制台浏览器地址: http://f.cncytech.com:9001/browser/cymdm61 或 /buckets/cymdm61/browse
// 3. 纯桶名: cymdm61
func parseBucketURL(raw string) (s3Base, consoleBase, bucketName string) {
	s3Base = "http://f.cncytech.com:9000"
	consoleBase = "http://f.cncytech.com:9001"
	bucketName = "cymdm62"

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}

	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		if strings.Contains(raw, "/") || strings.Contains(raw, ":") {
			raw = "http://" + raw
		} else {
			bucketName = strings.Trim(raw, "/")
			return
		}
	}

	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return
	}

	scheme := u.Scheme
	if scheme == "" {
		scheme = "http"
	}
	hostname := u.Hostname()
	port := u.Port()

	cleanPath := strings.Trim(u.Path, "/")
	parts := strings.Split(cleanPath, "/")

	isConsoleURL := false
	if len(parts) >= 2 && (parts[0] == "browser" || parts[0] == "buckets") {
		bucketName = parts[1]
		isConsoleURL = true
	} else if len(parts) >= 1 && parts[0] != "" {
		bucketName = parts[0]
	}

	if port == "9001" || isConsoleURL {
		cPort := port
		if cPort == "" {
			cPort = "9001"
		}
		consoleBase = fmt.Sprintf("%s://%s:%s", scheme, hostname, cPort)
		s3Port := "9000"
		if port != "" && port != "9001" {
			s3Port = port
		}
		s3Base = fmt.Sprintf("%s://%s:%s", scheme, hostname, s3Port)
	} else {
		if port != "" {
			s3Base = fmt.Sprintf("%s://%s:%s", scheme, hostname, port)
		} else {
			s3Base = fmt.Sprintf("%s://%s", scheme, hostname)
		}
		consoleBase = fmt.Sprintf("%s://%s:9001", scheme, hostname)
	}

	return
}

func readAgentHistory() agentHistoryFile {
	res := agentHistoryFile{
		Profiles: json.RawMessage("[]"),
		Buckets:  []string{DefaultRemoteBucket},
	}

	paths := []string{AgentHistoryPath, "history.json"}
	var data []byte
	for _, p := range paths {
		if b, err := os.ReadFile(p); err == nil && len(strings.TrimSpace(string(b))) > 0 {
			data = b
			break
		}
	}
	if len(data) == 0 {
		return res
	}

	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "[") {
		res.Profiles = json.RawMessage(trimmed)
		return res
	}

	var parsed agentHistoryFile
	if err := json.Unmarshal(data, &parsed); err == nil {
		if len(parsed.Profiles) > 0 {
			res.Profiles = parsed.Profiles
		}
		if len(parsed.Buckets) > 0 {
			res.Buckets = parsed.Buckets
		}
	}
	return res
}

func writeAgentHistory(hf agentHistoryFile) error {
	if len(hf.Profiles) == 0 {
		hf.Profiles = json.RawMessage("[]")
	}
	if len(hf.Buckets) == 0 {
		hf.Buckets = []string{DefaultRemoteBucket}
	}
	data, err := json.MarshalIndent(hf, "", "  ")
	if err != nil {
		return err
	}
	_ = os.WriteFile("history.json", data, 0644)
	return os.WriteFile(AgentHistoryPath, data, 0644)
}

func buildBucketItems(buckets []string) []gin.H {
	var items []gin.H
	for _, b := range buckets {
		s3Base, _, bName := parseBucketURL(b)
		items = append(items, gin.H{
			"url":     b,
			"name":    bName,
			"s3_url":  fmt.Sprintf("%s/%s/", s3Base, bName),
			"display": fmt.Sprintf("%s (%s)", bName, b),
		})
	}
	return items
}

func handleGetBuckets(c *gin.Context) {
	hf := readAgentHistory()
	c.JSON(200, gin.H{
		"buckets": hf.Buckets,
		"items":   buildBucketItems(hf.Buckets),
	})
}

func handleAddBucket(c *gin.Context) {
	var req struct {
		Bucket string `json:"bucket"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Bucket) == "" {
		c.JSON(400, gin.H{"error": "请输入有效的桶地址"})
		return
	}

	raw := strings.TrimSpace(req.Bucket)
	_, _, bName := parseBucketURL(raw)
	if bName == "" {
		c.JSON(400, gin.H{"error": "无法从输入地址中解析出桶名称"})
		return
	}

	hf := readAgentHistory()
	exists := false
	for _, existing := range hf.Buckets {
		if strings.EqualFold(strings.TrimRight(existing, "/"), strings.TrimRight(raw, "/")) {
			raw = existing
			exists = true
			break
		}
	}
	if !exists {
		hf.Buckets = append(hf.Buckets, raw)
		_ = writeAgentHistory(hf)
	}

	c.JSON(200, gin.H{
		"message": "桶地址已保存到 history.json",
		"current": raw,
		"buckets": hf.Buckets,
		"items":   buildBucketItems(hf.Buckets),
	})
}

func handleDeleteBucket(c *gin.Context) {
	target := strings.TrimSpace(c.Query("bucket"))
	if target == "" {
		var req struct {
			Bucket string `json:"bucket"`
		}
		_ = c.ShouldBindJSON(&req)
		target = strings.TrimSpace(req.Bucket)
	}

	hf := readAgentHistory()
	var updated []string
	for _, b := range hf.Buckets {
		if strings.TrimRight(b, "/") != strings.TrimRight(target, "/") {
			updated = append(updated, b)
		}
	}
	if len(updated) == 0 {
		updated = []string{DefaultRemoteBucket}
	}
	hf.Buckets = updated
	_ = writeAgentHistory(hf)

	c.JSON(200, gin.H{
		"buckets": hf.Buckets,
		"items":   buildBucketItems(hf.Buckets),
	})
}

// 获取远端指定桶的安装包列表 (支持用户自定义桶与 MinIO 9000/9001 双通道解析)
func handleRemotePackages(c *gin.Context) {
	type PkgInfo struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		Size     string `json:"size"`
		Modified string `json:"modified"`
	}

	rawBucket := c.Query("bucket")
	if strings.TrimSpace(rawBucket) == "" {
		rawBucket = DefaultRemoteBucket
	}
	s3Base, consoleBase, bucketName := parseBucketURL(rawBucket)
	s3BucketURL := fmt.Sprintf("%s/%s/", s3Base, bucketName)

	var pkgs []PkgInfo

	// 1. 优先尝试直接从 S3 端口 (如 9000) 获取 XML 列表
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Get(s3BucketURL)
	if err == nil && resp.StatusCode == 200 {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err == nil {
			var bucketResult S3ListBucketResult
			if err := xml.Unmarshal(body, &bucketResult); err == nil {
				for _, item := range bucketResult.Contents {
					if !strings.HasSuffix(item.Key, ".tar.gz") && !strings.HasSuffix(item.Key, ".zip") && !strings.HasSuffix(item.Key, ".tgz") {
						continue
					}
					sizeMB := float64(item.Size) / 1024 / 1024
					sizeStr := fmt.Sprintf("%.1f MB", sizeMB)
					if sizeMB >= 1024 {
						sizeStr = fmt.Sprintf("%.2f GB", sizeMB/1024)
					}
					pkgs = append(pkgs, PkgInfo{
						Name:     item.Key,
						URL:      fmt.Sprintf("%s/%s/%s", s3Base, bucketName, item.Key),
						Size:     sizeStr,
						Modified: item.LastModified,
					})
				}
				c.JSON(200, gin.H{"bucket": bucketName, "bucket_url": rawBucket, "packages": pkgs})
				return
			}
		}
	}

	// 2. 备用策略：使用 MinIO 控制台 9001 端口 API 登录 (admin / Nqsky1130) 并获取对象列表
	type loginReq struct {
		AccessKey string `json:"accessKey"`
		SecretKey string `json:"secretKey"`
	}
	loginPayload, _ := json.Marshal(loginReq{AccessKey: "admin", SecretKey: "Nqsky1130"})
	loginResp, err := client.Post(consoleBase+"/api/v1/login", "application/json", strings.NewReader(string(loginPayload)))
	if err == nil && (loginResp.StatusCode == 200 || loginResp.StatusCode == 204) {
		defer loginResp.Body.Close()
		var tokenCookie *http.Cookie
		for _, ck := range loginResp.Cookies() {
			if ck.Name == "token" {
				tokenCookie = ck
				break
			}
		}

		if tokenCookie != nil {
			req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/buckets/%s/objects", consoleBase, bucketName), nil)
			if err == nil {
				req.AddCookie(tokenCookie)
				objResp, err := client.Do(req)
				if err == nil && objResp.StatusCode == 200 {
					defer objResp.Body.Close()
					var result struct {
						Objects []struct {
							Name         string `json:"name"`
							Size         int64  `json:"size"`
							LastModified string `json:"last_modified"`
						} `json:"objects"`
					}
					if err := json.NewDecoder(objResp.Body).Decode(&result); err == nil {
						for _, item := range result.Objects {
							if !strings.HasSuffix(item.Name, ".tar.gz") && !strings.HasSuffix(item.Name, ".zip") && !strings.HasSuffix(item.Name, ".tgz") {
								continue
							}
							sizeMB := float64(item.Size) / 1024 / 1024
							sizeStr := fmt.Sprintf("%.1f MB", sizeMB)
							if sizeMB >= 1024 {
								sizeStr = fmt.Sprintf("%.2f GB", sizeMB/1024)
							}
							pkgs = append(pkgs, PkgInfo{
								Name:     item.Name,
								URL:      fmt.Sprintf("%s/%s/%s", s3Base, bucketName, item.Name),
								Size:     sizeStr,
								Modified: item.LastModified,
							})
						}
						c.JSON(200, gin.H{"bucket": bucketName, "bucket_url": rawBucket, "packages": pkgs})
						return
					}
				}
			}
		}
	}

	c.JSON(500, gin.H{"error": fmt.Sprintf("获取桶 [%s] 安装包列表失败，请确认桶名称是否存在或检查 MinIO 状态", bucketName)})
}

// 代理流式下载远程安装包到本地浏览器（绕过本地浏览器代理拦截）
func handleRemotePackageDownload(c *gin.Context) {
	targetURL := strings.TrimSpace(c.Query("url"))
	if targetURL == "" {
		c.String(400, "missing url parameter")
		return
	}

	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
	}
	resp, err := client.Get(targetURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
		}
		c.String(502, "下载远程安装包失败")
		return
	}
	defer resp.Body.Close()

	fileName := "package.tar.gz"
	if u, err := url.Parse(targetURL); err == nil {
		base := filepath.Base(u.Path)
		if base != "" && base != "." && base != "/" {
			fileName = base
		}
	}

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	c.Header("Content-Type", "application/octet-stream")
	if resp.ContentLength > 0 {
		c.Header("Content-Length", fmt.Sprintf("%d", resp.ContentLength))
	}
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, resp.Body)
}

// 查找 /root 目录下以 pn 开头的解压安装目录
func findPnDirInRoot() string {
	if b, err := os.ReadFile("/root/.pn_last_dir"); err == nil {
		saved := strings.TrimSpace(string(b))
		if saved != "" && strings.HasPrefix(strings.ToLower(filepath.Base(saved)), "pn") {
			if st, sErr := os.Stat(saved); sErr == nil && st.IsDir() {
				return filepath.Clean(saved)
			}
		}
	}
	entries, err := os.ReadDir("/root")
	if err != nil {
		return ""
	}
	fallbackPnDir := ""
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(strings.ToLower(name), "pn") {
			candidate := filepath.Join("/root", name)
			if fallbackPnDir == "" {
				fallbackPnDir = candidate
			}
			if st, sErr := os.Stat(filepath.Join(candidate, InstallScript)); sErr == nil && !st.IsDir() {
				return candidate
			}
		}
	}
	return fallbackPnDir
}

// --- 目录检测 ---
func handleCheckDir(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	mode := c.Query("mode")
	useLast := c.Query("use_last")
	if mode == "pn" && (useLast == "1" || path == "") {
		if found := findPnDirInRoot(); found != "" {
			path = found
		}
	}
	cleanPath := filepath.Clean(path)

	res := map[string]interface{}{
		"exists":          false,
		"has_install":     false,
		"has_mdm":         false,
		"has_uem":         false,
		"has_webui":       false,
		"has_tomcat":      false,
		"has_backend":     false,
		"is_installed":    false,
		"is_pn_installed": false,
		"has_sys_webui":   false,
		"has_sys_tomcat":  false,
		"has_sys_backend": false,
		"resolved_path":   cleanPath,
		"debug_msg":       "",
	}

	// 检查服务器系统环境是否已经安装了对应模块
	if _, err := os.Stat("/opt/emm/current"); err == nil {
		res["is_installed"] = true
	}
	if info, err := os.Stat("/srv/hpns"); err == nil && info.IsDir() {
		res["is_pn_installed"] = true
	}
	if info, err := os.Stat("/opt/emm/current/source/WebUI"); err == nil && info.IsDir() {
		res["has_sys_webui"] = true
	}
	if info, err := os.Stat("/opt/emm/current/tomcat"); err == nil && info.IsDir() {
		res["has_sys_tomcat"] = true
	}
	if _, err := os.Stat("/opt/emm/current/tomcat/webapps/mdm.war"); err == nil {
		res["has_sys_backend"] = true
	}

	if cleanPath == "" || cleanPath == "." {
		res["debug_msg"] = "路径为空"
		c.JSON(200, res)
		return
	}

	info, err := os.Stat(cleanPath)
	if (err != nil || !info.IsDir()) && mode == "pn" {
		if found := findPnDirInRoot(); found != "" {
			if st, sErr := os.Stat(found); sErr == nil && st.IsDir() {
				cleanPath = found
				res["resolved_path"] = cleanPath
				info = st
				err = nil
			}
		}
	}
	if err != nil || !info.IsDir() {
		res["debug_msg"] = fmt.Sprintf("目录不存在: %s", cleanPath)
		c.JSON(200, res)
		return
	}
	res["exists"] = true

	files, err := os.ReadDir(cleanPath)
	if err == nil {
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			if name == InstallScript {
				res["has_install"] = true
			} else if strings.HasPrefix(name, "UEM-") && strings.HasSuffix(name, ".tar.gz") {
				res["has_uem"] = true
				res["has_mdm"] = true
			} else if strings.HasPrefix(strings.ToLower(name), "webui") && (strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") || strings.HasSuffix(name, ".zip")) {
				res["has_webui"] = true
				res["has_mdm"] = true
			} else if strings.HasPrefix(strings.ToLower(name), "tomcat") && (strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") || strings.HasSuffix(name, ".zip")) {
				res["has_tomcat"] = true
				res["has_mdm"] = true
			} else if name == "mdm.war" || strings.HasSuffix(name, ".war") {
				res["has_backend"] = true
				res["has_mdm"] = true
			} else if name == UpdateScript {
				res["has_mdm"] = true
			}
		}
		// 如果当前目录没有直接包含 install.sh，检查一级子目录中是否存在 install.sh (PN模式下仅匹配 pn 开头目录)
		if res["has_install"] == false {
			for _, f := range files {
				if f.IsDir() {
					if mode == "pn" && !strings.HasPrefix(strings.ToLower(f.Name()), "pn") {
						continue
					}
					subInstall := filepath.Join(cleanPath, f.Name(), InstallScript)
					if st, sErr := os.Stat(subInstall); sErr == nil && !st.IsDir() {
						res["has_install"] = true
						res["resolved_path"] = filepath.Join(cleanPath, f.Name())
						break
					}
				}
			}
		}
	}

	if res["has_mdm"] == false && res["has_install"] == false {
		res["debug_msg"] = fmt.Sprintf("未检测到安装包或更新包")
	}

	c.JSON(200, res)
}

// --- RPM & ISO ---
func handleRpmInstall(c *gin.Context) {
	c.Header("Content-Type", "text/plain")
	c.Writer.Flush()

	_, _ = fmt.Fprintf(c.Writer, ">>> Upload...\n")
	c.Writer.Flush()

	file, err := c.FormFile("file")
	if err != nil {
		return
	}
	p := filepath.Join(RpmCacheDir, file.Filename)
	_ = c.SaveUploadedFile(file, p)

	_, _ = fmt.Fprintf(c.Writer, ">>> Install...\n")
	c.Writer.Flush()

	cmd := exec.Command("rpm", "-Uvh", "--replacepkgs", p)
	s, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	_ = cmd.Start()

	sc := bufio.NewScanner(s)
	for sc.Scan() {
		_, _ = fmt.Fprintln(c.Writer, sc.Text())
		c.Writer.Flush()
	}
	_ = cmd.Wait()
	_, _ = fmt.Fprintf(c.Writer, "Done.\n")
}

func handleIsoMount(c *gin.Context) {
	c.Header("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(c.Writer, ">>> Upload ISO...\n")
	c.Writer.Flush()

	file, err := c.FormFile("file")
	if err != nil {
		return
	}
	_ = c.SaveUploadedFile(file, IsoSavePath)
	mountAndConfigRepo(c.Writer, IsoSavePath)
}

func handleIsoMountLocal(c *gin.Context) {
	c.Header("Content-Type", "text/plain; charset=utf-8")
	path := c.PostForm("path")
	_, _ = fmt.Fprintf(c.Writer, ">>> Checking: %s\n", path)
	c.Writer.Flush()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		_, _ = fmt.Fprintf(c.Writer, "Not Found\n")
		return
	}
	mountAndConfigRepo(c.Writer, path)
}

func mountAndConfigRepo(w io.Writer, isoPath string) {
	_, _ = fmt.Fprintf(w, ">>> Mounting...\n")
	_ = os.MkdirAll(IsoMountPoint, 0755)
	_ = exec.Command("umount", IsoMountPoint).Run()
	if out, err := exec.Command("mount", "-o", "loop", isoPath, IsoMountPoint).CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(w, "Fail: %s\n", out)
		return
	}
	_ = os.MkdirAll(RepoBackupDir, 0755)
	_ = exec.Command("bash", "-c", fmt.Sprintf("mv /etc/yum.repos.d/*.repo %s/", RepoBackupDir)).Run()
	rc := ""
	if _, err := os.Stat(filepath.Join(IsoMountPoint, "BaseOS")); err == nil {
		rc += fmt.Sprintf("[L-Base]\nname=Base\nbaseurl=file://%s/BaseOS\ngpgcheck=0\nenabled=1\n", IsoMountPoint)
	}
	if _, err := os.Stat(filepath.Join(IsoMountPoint, "AppStream")); err == nil {
		rc += fmt.Sprintf("[L-App]\nname=App\nbaseurl=file://%s/AppStream\ngpgcheck=0\nenabled=1\n", IsoMountPoint)
	}
	if rc == "" {
		rc = fmt.Sprintf("[L-ISO]\nname=ISO\nbaseurl=file://%s\ngpgcheck=0\nenabled=1\n", IsoMountPoint)
	}
	_ = os.WriteFile("/etc/yum.repos.d/local.repo", []byte(rc), 0644)
	_, _ = fmt.Fprintf(w, ">>> Yum makecache...\n")

	c := exec.Command("bash", "-c", "yum clean all && yum makecache")
	s, _ := c.StdoutPipe()
	c.Stderr = c.Stdout
	_ = c.Start()
	sc := bufio.NewScanner(s)
	for sc.Scan() {
		_, _ = fmt.Fprintln(w, sc.Text())
	}
	_ = c.Wait()
	_, _ = fmt.Fprintf(w, "Done.\n")
}
