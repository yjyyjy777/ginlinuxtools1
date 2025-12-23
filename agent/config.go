package main

import (
	"log"
	"os"

	"github.com/magiconair/properties"
)

// ================= 1. 配置区域 =================
var (
	ServerPort      string
	UploadTargetDir = "/root"
	RpmCacheDir     = "/root/rpm_cache"
	InstallWorkDir  = "/root/install-cncy" // 默认工作目录
	InstallScript   = "install.sh"
	UpdateScript    = "update.sh"
	IsoSavePath     = "/root/os.iso"
	IsoMountPoint   = "/mnt/cdrom"
	RepoBackupDir   = "/etc/yum.repos.d/backup_cncy"

	// MinioEndpoint MinIO API
	MinioEndpoint = "127.0.0.1:9000"
	MinioUser     = "admin"
	MinioPass     = "Nqsky1130"
	MinioBucket   = "nqsky"
)

var uemServices = []string{
	"tomcat", "Platform_java", "licserver", "AppServer", "EMMBackend",
	"nginx", "redis", "mysqld", "minio", "rabbitmq-server", "scep-go",
}

var logFileMap = map[string]string{
	"tomcat":      "/opt/emm/current/tomcat/logs/catalina.out",
	"app_server":  "/emm/logs/AppServer/appServer.log",
	"emm_backend": "/emm/logs/emm_backend/emmBackend.log",
	"license":     "/emm/logs/licenseServer/licenseServer.log",
	"platform":    "/emm/logs/platform/platform.log",
}

// Config --- Struct Definitions ---
type Config struct {
	RedisHost           string `properties:"system.redis.host"`
	RedisPort           int    `properties:"system.redis.port"`
	RedisPassword       string `properties:"system.redis.password"`
	MdmJdbcURL          string `properties:"jdbc.url"`
	MdmJdbcUsername     string `properties:"jdbc.username"`
	MdmJdbcPassword     string `properties:"jdbc.password"`
	MtenantJdbcURL      string `properties:"jdbc.multitenant.url"`
	MtenantJdbcUsername string `properties:"jdbc.multitenant.username"`
	MtenantJdbcPassword string `properties:"jdbc.multitenant.password"`
	RabbitMQAddresses   string `properties:"spring.rabbitmq.addresses"`
	RabbitMQAdminPort   int    `properties:"rabbitmq.admin.port,default=15672"`
	RabbitMQUsername    string `properties:"spring.rabbitmq.username"`
	RabbitMQPassword    string `properties:"spring.rabbitmq.password"`
	MinioURL            string `properties:"storage.minio.url"`
}

type Metric struct {
	Time            int64  `json:"time"`
	Uptime          int64  `json:"uptime"`
	UptimeStr       string `json:"uptime_str"`
	Threads         int    `json:"threads"`
	QPS             int    `json:"qps"`
	MaxConnections  int    `json:"max_connections"`
	SlowQueries     int    `json:"slow_queries"`
	OpenTables      int    `json:"open_tables"`
	InnoDBBuffUsed  int    `json:"innodb_buff_used"`
	InnoDBBuffTotal int    `json:"innodb_buff_total"`
}

type TableStat struct {
	Name   string `json:"name"`
	Rows   int    `json:"rows"`
	SizeMB int    `json:"size_mb"`
	Ops    int    `json:"ops"`
}

type ProcessListRow struct {
	Id      int    `json:"id"`
	User    string `json:"user"`
	Host    string `json:"host"`
	DB      string `json:"db"`
	Command string `json:"command"`
	Time    int    `json:"time"`
	State   string `json:"state"`
	Info    string `json:"info"`
}

type ReplicationStatus struct {
	Role          string `json:"role"`
	SlaveRunning  bool   `json:"slave_running"`
	SecondsBehind int    `json:"seconds_behind"`
}

type SqlResult struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
	Error   string     `json:"error,omitempty"`
}

// [ADDED] ProcessInfo for top-like monitoring
type ProcessInfo struct {
	PID     string `json:"pid"`
	User    string `json:"user"`
	CPU     string `json:"cpu"`
	Mem     string `json:"mem"`
	Command string `json:"command"`
}

// DiskInfo System Info Structs
type DiskInfo struct {
	Mount string `json:"mount"`
	Total string `json:"total"`
	Used  string `json:"used"`
	Usage int    `json:"usage"`
}

type NetstatInfo struct {
	Proto   string `json:"proto"`
	Address string `json:"address"`
	Pid     string `json:"pid"`
}

type SysInfo struct {
	CpuCores     int           `json:"cpu_cores"`
	CpuPass      bool          `json:"cpu_pass"`
	MemTotal     string        `json:"mem_total"`
	MemPass      bool          `json:"mem_pass"`
	Arch         string        `json:"arch"`
	OsName       string        `json:"os_name"`
	OsPass       bool          `json:"os_pass"`
	DiskList     []DiskInfo    `json:"disk_list"`
	DiskDetail   string        `json:"disk_detail"`
	Ulimit       string        `json:"ulimit"`
	UlimitPass   bool          `json:"ulimit_pass"`
	MemUsage     float64       `json:"mem_usage"`
	LoadAvg      float64       `json:"load_avg"`
	NetRx        float64       `json:"net_rx"`
	NetTx        float64       `json:"net_tx"`
	Netstat      []NetstatInfo `json:"netstat"`
	TcpConnCount int           `json:"tcp_conn_count"`
	TopCPUProcs  []ProcessInfo `json:"top_cpu_procs"` // [ADDED]
	TopMemProcs  []ProcessInfo `json:"top_mem_procs"` // [ADDED]
	RootCrontab  []string      `json:"root_crontab"`  // [ADDED]
}
type SecInfo struct {
	SELinux     string `json:"selinux"`
	Firewall    string `json:"firewall"`
	SshTunnelOk bool   `json:"ssh_tunnel_ok"`
}
type ServiceStat struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}
type UemInfo struct {
	Installed bool          `json:"installed"`
	Services  []ServiceStat `json:"services"`
}
type MinioInfo struct {
	BucketExists bool   `json:"bucket_exists"`
	Policy       string `json:"policy"`
}
type FullCheckResult struct {
	SysInfo   SysInfo   `json:"sys_info"`
	SecInfo   SecInfo   `json:"sec_info"`
	UemInfo   UemInfo   `json:"uem_info"`
	MinioInfo MinioInfo `json:"minio_info"`
}
type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    string `json:"size"`
	ModTime string `json:"mod_time"`
}
type WSMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

var appConfig Config

func loadConfig() {
	prodPath := "/opt/emm/current/config/global.properties"
	localPath := "global.properties"
	var p *properties.Properties
	var err error
	if _, err = os.Stat(prodPath); err == nil {
		log.Printf("Loading config: %s", prodPath)
		p, err = properties.LoadFile(prodPath, properties.UTF8)
	} else {
		log.Printf("Loading local config: %s", localPath)
		p, err = properties.LoadFile(localPath, properties.UTF8)
	}
	if err != nil {
		log.Printf("Warning: Config error: %v", err)
		return
	}
	if err := p.Decode(&appConfig); err != nil {
		log.Printf("Warning: Decode error: %v", err)
	}
}
