package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// --- 目录检测 ---
func handleCheckDir(c *gin.Context) {
	path := c.Query("path")
	cleanPath := filepath.Clean(path)

	res := map[string]interface{}{
		"exists":      false,
		"has_install": false,
		"has_mdm":     false,
		"debug_msg":   "",
	}

	if cleanPath == "" || cleanPath == "." {
		res["debug_msg"] = "路径为空"
		c.JSON(200, res)
		return
	}

	info, err := os.Stat(cleanPath)
	if err != nil || !info.IsDir() {
		res["debug_msg"] = fmt.Sprintf("目录不存在: %s", cleanPath)
		c.JSON(200, res)
		return
	}
	res["exists"] = true

	if _, err := os.Stat(filepath.Join(cleanPath, InstallScript)); err == nil {
		res["has_install"] = true
	}

	// [MODIFIED] Granular check for update scripts
	if _, err := os.Stat(filepath.Join(cleanPath, "update.sh")); err == nil {
		res["has_uem"] = true
		res["has_mdm"] = true // Keep based on update.sh for now
	} else if _, err := os.Stat(filepath.Join(cleanPath, UpdateScript)); err == nil {
		res["has_uem"] = true
		res["has_mdm"] = true
	}

	if _, err := os.Stat(filepath.Join(cleanPath, "update_webui.sh")); err == nil {
		res["has_webui"] = true
		res["has_mdm"] = true
	}

	if _, err := os.Stat(filepath.Join(cleanPath, "update_tomcat.sh")); err == nil {
		res["has_tomcat"] = true
		res["has_mdm"] = true
	}

	if res["has_mdm"] == false {
		res["debug_msg"] = fmt.Sprintf("未找到更新脚本 (update.sh, update_webui.sh, etc)")
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
