package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// --- 部署 WebSocket ---
func handleDeployWS(c *gin.Context) {
	workDir := c.Query("path")
	deployType := c.Query("type")
	// scriptArg := c.Query("arg") // Deprecated or less used with auto-detection but kept if needed

	// [MODIFIED] If path is empty, default to InstallWorkDir
	if workDir == "" {
		workDir = InstallWorkDir
	}

	var cmd *exec.Cmd

	if deployType == "wget" {
		url := c.Query("url")
		extract := c.Query("extract")
		mode := c.Query("mode")
		if url == "" {
			c.Writer.Write([]byte("Error: url is empty"))
			return
		}
		// 生成下载并可选解压脚本
		// 如果extract=1，下载完成后在 workDir 下执行解压并自动 cd 到解压目录
		bashCmd := fmt.Sprintf("cd %s && echo '>>> 开始下载: %s' && wget -c '%s'", workDir, url, url)
		if extract == "1" || extract == "true" {
			recordCmd := ""
			if mode == "pn" {
				recordCmd = "pwd > /root/.pn_last_dir; "
			}
			bashCmd += fmt.Sprintf(" && PKG_FILE=$(basename '%s') && echo '>>> 开始解压安装包...' && tar -zxvf \"$PKG_FILE\" -C %s && INSTALL_SH=$(tar -ztf \"$PKG_FILE\" | sed 's|^\\./||' | grep -E '(^|/)install\\.sh$' | head -n 1) && if [ -n \"$INSTALL_SH\" ]; then REL_DIR=$(dirname \"$INSTALL_SH\"); if [ \"$REL_DIR\" = \".\" ]; then EXTRACT_DIR=\"%s\"; else EXTRACT_DIR=\"%s/$REL_DIR\"; fi; else TOP_DIR=$(tar -ztf \"$PKG_FILE\" | sed 's|^\\./||' | head -n 1 | cut -d/ -f1); EXTRACT_DIR=\"%s/$TOP_DIR\"; fi && if [ -d \"$EXTRACT_DIR\" ]; then cd \"$EXTRACT_DIR\" && %secho \">>> 已进入解压目录: $(pwd)\"; fi && echo '>>> 下载与解压完成！'", url, workDir, workDir, workDir, workDir, recordCmd)
		} else {
			bashCmd += " && echo '>>> 下载完成！'"
		}
		cmd = exec.Command("/bin/bash", "-c", bashCmd)
		cmd.Dir = workDir
	} else if deployType == "install" {
		cmd = exec.Command("/bin/bash", filepath.Join(workDir, InstallScript))
		cmd.Dir = workDir
	} else {
		// [MODIFIED] Write embedded update.sh to workDir
		updateScriptPath := filepath.Join(workDir, "update.sh")
		if err := os.WriteFile(updateScriptPath, embeddedUpdateScript, 0755); err != nil {
			c.Writer.Write([]byte("Error writing update.sh: " + err.Error()))
			return
		}

		scriptArg := c.Query("arg")
		if scriptArg == "" {
			scriptArg = "uem" // Default fallback
		}

		dbMode := c.Query("db_mode")
		if dbMode == "" {
			dbMode = "keep-data"
		}

		var cmdArgs []string
		targetScript := "update.sh"

		switch scriptArg {
		case "uem":
			cmdArgs = []string{targetScript, "uem", dbMode}
		case "webui":
			cmdArgs = []string{targetScript, "webui"}
			if c.Query("local") == "1" || c.Query("local") == "true" {
				cmdArgs = append(cmdArgs, "local")
			}
		case "backend", "war":
			cmdArgs = []string{targetScript, "backend"}
		case "tomcat":
			cmdArgs = []string{targetScript, "tomcat", "local"}
		default:
			cmdArgs = []string{targetScript, scriptArg}
		}
		
		cmd = exec.Command("/bin/bash", cmdArgs...)
		cmd.Dir = workDir
	}

	startPTYSession(c.Writer, c.Request, cmd)
}

func handleLogWS(c *gin.Context) {
	key := c.Query("key")
	path, ok := logFileMap[key]
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer func(conn *websocket.Conn) {
		_ = conn.Close()
	}(conn)
	if !ok {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("Bad Key"))
		return
	}
	_ = os.Chmod(path, 0644)
	cmd := exec.Command("tail", "-f", "-n", "200", path)
	out, _ := cmd.StdoutPipe()
	_ = cmd.Start()
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	buf := make([]byte, 4096)
	for {
		n, err := out.Read(buf)
		if err != nil {
			break
		}
		valid := strings.ToValidUTF8(string(buf[:n]), "")
		if conn.WriteMessage(1, []byte(valid)) != nil {
			break
		}
	}
}

// --- Terminals ---
func handleSysTermWS(c *gin.Context) {
	startPTYSession(c.Writer, c.Request, exec.Command("/bin/bash"))
}

func startPTYSession(w http.ResponseWriter, r *http.Request, c *exec.Cmd) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func(conn *websocket.Conn) {
		_ = conn.Close()
	}(conn)

	ptmx, tty, err := pty.Open()
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("Err:"+err.Error()))
		return
	}
	defer func(tty *os.File) {
		_ = tty.Close()
	}(tty)
	defer func(ptmx *os.File) {
		_ = ptmx.Close()
	}(ptmx)

	c.Env = append(os.Environ(), "TERM=xterm-256color")
	c.Stdout = tty
	c.Stdin = tty
	c.Stderr = tty
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}
	c.SysProcAttr.Setsid = true
	c.SysProcAttr.Setctty = true

	if err := c.Start(); err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("Start Err:"+err.Error()))
		return
	}
	defer func() {
		_ = c.Process.Kill()
		_ = c.Wait()
	}()

	go func() {
		_ = c.Wait()
		time.Sleep(300 * time.Millisecond)
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[PROCESS_COMPLETED]\r\n"))
		_ = conn.Close()
	}()

	go func() {
		buf := make([]byte, 2048)
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				return
			}
			if conn.WriteMessage(websocket.TextMessage, buf[:n]) != nil {
				return
			}
		}
	}()

	for {
		_, m, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var msg WSMessage
		if json.Unmarshal(m, &msg) == nil {
			if msg.Type == "input" {
				_, _ = ptmx.Write([]byte(msg.Data))
			} else if msg.Type == "resize" {
				_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(msg.Rows), Cols: uint16(msg.Cols)})
			}
		}
	}
}
