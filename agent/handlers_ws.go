package main

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/creack/pty"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// --- 部署 WebSocket ---
func handleDeployWS(c *gin.Context) {
	workDir := c.Query("path")
	deployType := c.Query("type")
	// scriptArg := c.Query("arg") // Deprecated or less used with auto-detection but kept if needed

	// [MODIFIED] If path is empty, default to InstallWorkDir (or /root based on new logic if needed)
	if workDir == "" {
		workDir = "/root"
	}

	var cmd *exec.Cmd

	if deployType == "install" {
		cmd = exec.Command("/bin/bash", filepath.Join(InstallWorkDir, InstallScript))
	} else {
		// [MODIFIED] Dispatch based on 'arg' from frontend (uem, webui, tomcat)
		// Frontend calls: startScript('update', 'uem') -> arg=uem
		scriptArg := c.Query("arg")
		if scriptArg == "" {
			scriptArg = "uem" // Default fallback
		}

		targetScript := ""
		switch scriptArg {
		case "uem":
			if _, err := os.Stat(filepath.Join(workDir, "update.sh")); err == nil {
				targetScript = "update.sh"
			} else {
				targetScript = UpdateScript // Fallback to config default (mdm.sh/update.sh)
			}
		case "webui":
			targetScript = "update_webui.sh"
		case "tomcat":
			targetScript = "update_tomcat.sh"
		default:
			targetScript = "update.sh"
		}

		// Execute the chosen script
		// Note: update.sh might need 'uem' arg if it's the old mdm.sh style, but user said "update.sh is for updating mdm", imply no arg needed or arg is implicit.
		// However, the legacy mdm.sh took 'uem', 'webui' etc.
		// If the new 'update.sh' is a direct replacement for 'mdm.sh', it might still accept args.
		// But 'update_tomcat.sh' likely doesn't need 'tomcat' arg.

		if targetScript == "update.sh" || targetScript == "mdm.sh" {
			// For the main update script, we might pass the arg if it expects it.
			// But if we have specific scripts (update_tomcat.sh), we don't use update.sh for them.
			// For 'uem' case, we run update.sh.
			cmd = exec.Command("/bin/bash", targetScript)
		} else {
			cmd = exec.Command("/bin/bash", targetScript)
		}
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
