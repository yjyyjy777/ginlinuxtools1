package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	flag.StringVar(&ServerPort, "port", "9898", "Server listening port")
	flag.Parse()

	// 初始化环境
	_ = os.MkdirAll(RpmCacheDir, 0755)
	_ = autoFixSshConfig()

	initLogPaths()
	loadConfig()
	initRedis()
	initMySQL()

	// Gin 初始化
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// 1. 静态首页 (使用新的模板渲染 handler)
	r.GET("/", handleIndex)

	// 2. 文件上传
	r.POST("/upload", handleUpload)
	r.POST("/api/upload_any", handleUploadAny)

	// 3. API 组
	api := r.Group("/api")
	{
		// 系统 & 文件
		api.GET("/fs/list", handleFsList)
		api.GET("/fs/download", handleFsDownload)
		api.GET("/check", handleCheckEnv)
		api.POST("/service/restart", handleRestartService)
		api.POST("/minio/fix", handleFixMinio)
		api.POST("/fix_ssh", handleFixSsh)
		api.POST("/sec/selinux", handleFixSelinux)
		api.POST("/sec/firewall", handleFixFirewall)
		api.GET("/log/download", handleLogDownload)

		// 部署 & 依赖
		api.POST("/rpm_install", handleRpmInstall)
		api.POST("/iso_mount", handleIsoMount)
		api.POST("/iso_mount_local", handleIsoMountLocal)
		api.GET("/check_dir", handleCheckDir)
	}

	// 4. WebSocket 组
	ws := r.Group("/ws")
	{
		ws.GET("/deploy", handleDeployWS)
		ws.GET("/terminal", handleSysTermWS)
		ws.GET("/log", handleLogWS)
	}

	// 5. 基础服务 (Base Services) 路由 & 代理
	bs := api.Group("/baseservices")
	{
		bs.GET("/redis/keys", redisKeysAndTypesHandler)
		bs.GET("/redis/info", redisInfoHandler)
		bs.DELETE("/redis/key", redisKeyHandler)
		bs.Any("/redis/value", redisValueHandler)

		bs.GET("/mysql/metrics/*db", apiMetrics)
		bs.GET("/mysql/tables/*db", apiTables)
		bs.GET("/mysql/processlist/*db", apiProcesslist)
		bs.GET("/mysql/replstatus/*db", apiRepl)
		bs.POST("/mysql/execsql/*db", executeSQL)
	}

	// 设置反向代理 (RabbitMQ 和 MinIO)
	setupGinProxies(r)

	fmt.Printf("Agent running on %s (Gin Framework)\n", ServerPort)
	if err := r.Run(":" + ServerPort); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %s\n", err)
	}
}
