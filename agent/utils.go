package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func getMemTotalKB() uint64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)
	s := bufio.NewScanner(f)
	for s.Scan() {
		if strings.HasPrefix(s.Text(), "MemTotal:") {
			var k uint64
			_, _ = fmt.Sscanf(strings.Fields(s.Text())[1], "%d", &k)
			return k
		}
	}
	return 0
}

func getMemAvailableKB() uint64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)
	s := bufio.NewScanner(f)
	for s.Scan() {
		if strings.HasPrefix(s.Text(), "MemAvailable:") {
			var k uint64
			_, _ = fmt.Sscanf(strings.Fields(s.Text())[1], "%d", &k)
			return k
		}
	}
	return 0
}

func getLoadAvg() float64 {
	d, _ := os.ReadFile("/proc/loadavg")
	if len(d) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(strings.Fields(string(d))[0], 64)
	return v
}

func getOSName() string {
	f, _ := os.Open("/etc/os-release")
	defer func(f *os.File) {
		_ = f.Close()
	}(f)
	s := bufio.NewScanner(f)
	n, v := "", ""
	for s.Scan() {
		l := s.Text()
		if strings.HasPrefix(l, "NAME=") {
			n = strings.Trim(strings.Split(l, "=")[1], "\"")
		}
		if strings.HasPrefix(l, "VERSION=") {
			v = strings.Trim(strings.Split(l, "=")[1], "\"")
		}
	}
	return n + " " + v
}

func getNetIO() (float64, float64) {
	d, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	lines := strings.Split(string(d), "\n")
	var rx, tx uint64
	for _, l := range lines {
		f := strings.Fields(l)
		if len(f) < 10 {
			continue
		}
		if strings.Contains(f[0], ":") || len(f) > 16 {
			rStr := f[1]
			tStr := f[9]
			if strings.Contains(f[0], ":") && len(f) < 17 {
				parts := strings.Split(f[0], ":")
				if len(parts) > 1 {
					rStr = parts[1]
				}
			}
			r, _ := strconv.ParseUint(rStr, 10, 64)
			t, _ := strconv.ParseUint(tStr, 10, 64)
			rx += r
			tx += t
		}
	}
	now := time.Now()
	rRate, tRate := 0.0, 0.0
	netMutex.Lock()
	if !lastNetTime.IsZero() {
		sec := now.Sub(lastNetTime).Seconds()
		if sec > 0 {
			rRate = float64(rx-lastNetRx) / sec / 1024
			tRate = float64(tx-lastNetTx) / sec / 1024
		}
	}
	lastNetRx = rx
	lastNetTx = tx
	lastNetTime = now
	netMutex.Unlock()
	return rRate, tRate
}

func formatBytes(b int64) string {
	const u = 1024
	if b < u {
		return fmt.Sprintf("%dB", b)
	}
	d, e := int64(u), 0
	for n := b / u; n >= u; n /= u {
		d *= u
		e++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(d), "KMGTPE"[e])
}

func checkSshConfig() bool {
	d, err := os.ReadFile("/etc/ssh/sshd_config")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(d), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 && strings.EqualFold(fields[0], "AllowTcpForwarding") && strings.EqualFold(fields[1], "yes") {
			return true
		}
	}
	return false
}

func autoFixSshConfig() error {
	if checkSshConfig() {
		return nil
	}
	const cfgPath = "/etc/ssh/sshd_config"
	contentBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("read sshd_config failed: %w", err)
	}
	content := string(contentBytes)
	if strings.Contains(content, "#AllowTcpForwarding yes") {
		content = strings.ReplaceAll(content, "#AllowTcpForwarding yes", "AllowTcpForwarding yes")
	} else {
		content += "\nAllowTcpForwarding yes\n"
	}
	err = os.WriteFile(cfgPath, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("write sshd_config failed: %w", err)
	}
	return exec.Command("systemctl", "restart", "sshd").Run()
}
