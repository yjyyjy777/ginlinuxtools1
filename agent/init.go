package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/websocket"
)

var (
	rdb           *redis.Client
	dbConnections = make(map[string]*sql.DB)
	dbConnMutex   sync.RWMutex
	ctx           = context.Background()
	lastQuestions int64
	lastQTime     time.Time
	qpsMutex      sync.Mutex
	lastNetRx     uint64
	lastNetTx     uint64
	lastNetTime   time.Time
	netMutex      sync.Mutex
	upgrader      = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
)

func initLogPaths() {
	resolveLog := func(primary, fallback string) string {
		if _, err := os.Stat(primary); err == nil {
			return primary
		}
		if _, err := os.Stat(fallback); err == nil {
			return fallback
		}
		return primary
	}
	logFileMap["nginx_access"] = resolveLog("/var/log/nginx/access.log", "/usr/local/nginx/logs/access.log")
	logFileMap["nginx_error"] = resolveLog("/var/log/nginx/error.log", "/usr/local/nginx/logs/error.log")
}

func initRedis() {
	if appConfig.RedisHost == "" {
		log.Println("Redis skipped.")
		return
	}
	addr := fmt.Sprintf("%s:%d", appConfig.RedisHost, appConfig.RedisPort)
	client := redis.NewClient(&redis.Options{
		Addr:        addr,
		Password:    appConfig.RedisPassword,
		DB:          0,
		DialTimeout: 2 * time.Second,
		ReadTimeout: 2 * time.Second,
	})
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := client.Ping(pingCtx).Result(); err != nil {
		log.Printf("Redis connect fail: %v", err)
		_ = client.Close()
		rdb = nil
	} else {
		rdb = client
		log.Println("Redis connected.")
	}
}

func initMySQL() {
	if appConfig.MdmJdbcURL == "" {
		log.Println("MySQL skipped.")
		return
	}
	configs := map[string]map[string]string{
		"mdm":         {"url": appConfig.MdmJdbcURL, "username": appConfig.MdmJdbcUsername, "password": appConfig.MdmJdbcPassword},
		"multitenant": {"url": appConfig.MtenantJdbcURL, "username": appConfig.MtenantJdbcUsername, "password": appConfig.MtenantJdbcPassword},
	}
	for dbName, config := range configs {
		// [防御性编程] 使用匿名函数和 defer/recover 来捕获任何可能的 panic
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("FATAL: Panic recovered during MySQL init for [%s]: %v", dbName, r)
				}
			}()

			var dsn string
			if temp := strings.Split(config["url"], "//"); len(temp) > 1 {
				parts := strings.Split(temp[1], "/")
				if len(parts) > 1 {
					hostAndPort, dbNameAndParams := parts[0], parts[1]
					dbNameFromURL := strings.Split(dbNameAndParams, "?")[0]
					dsn = fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true&timeout=2s", config["username"], config["password"], hostAndPort, dbNameFromURL)
				}
			}
			if dsn == "" {
				log.Printf("DSN is empty for %s, skipping.", dbName)
				return // 使用 return 代替 continue
			}

			db, err := sql.Open("mysql", dsn)
			if err != nil {
				log.Printf("MySQL %s open error: %v", dbName, err)
				return // 使用 return 代替 continue
			}

			db.SetConnMaxLifetime(time.Minute * 3)
			db.SetMaxOpenConns(10)
			db.SetMaxIdleConns(5)

			if err = db.Ping(); err != nil {
				log.Printf("MySQL %s ping error: %v", dbName, err)
				// 即使 ping 失败，也确保关闭 db 对象以释放资源
				_ = db.Close()
				return // 使用 return 代替 continue
			}

			dbConnMutex.Lock()
			dbConnections[dbName] = db
			dbConnMutex.Unlock()
			log.Printf("MySQL %s connected", dbName)
		}()
	}
}
