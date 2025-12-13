package main

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// --- Redis Handlers ---
func redisKeysAndTypesHandler(c *gin.Context) {
	if rdb == nil {
		c.String(503, "Redis not connected")
		return
	}
	var cursor uint64
	var allKeys []string
	for {
		var keys []string
		var err error
		var nextCursor uint64
		// FIX: Use different variable name for returned cursor to avoid shadowing 'c' (*gin.Context)
		keys, nextCursor, err = rdb.Scan(c, cursor, "*", 500).Result()
		if err != nil {
			c.String(500, err.Error())
			return
		}
		allKeys = append(allKeys, keys...)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	if len(allKeys) == 0 {
		c.JSON(200, []string{})
		return
	}
	pipe := rdb.Pipeline()
	keyTypes := make([]*redis.StatusCmd, len(allKeys))
	for i, key := range allKeys {
		keyTypes[i] = pipe.Type(c, key)
	}
	_, _ = pipe.Exec(c)
	result := make([]map[string]string, len(allKeys))
	for i, key := range allKeys {
		result[i] = map[string]string{"key": key, "type": keyTypes[i].Val()}
	}
	c.JSON(200, result)
}

func redisValueHandler(c *gin.Context) {
	if rdb == nil {
		c.String(503, "Redis not connected")
		return
	}
	key := c.Query("key")
	dataType := c.Query("type")

	if c.Request.Method == "GET" {
		if key == "" || dataType == "" {
			c.String(400, "Missing params")
			return
		}
		var val interface{}
		var err error
		switch dataType {
		case "string":
			val, err = rdb.Get(c, key).Result()
		case "list":
			val, err = rdb.LRange(c, key, 0, -1).Result()
		case "hash":
			val, err = rdb.HGetAll(c, key).Result()
		default:
			c.String(400, "Unsupported type")
			return
		}
		if err != nil {
			c.String(500, err.Error())
			return
		}
		c.JSON(200, map[string]interface{}{"key": key, "type": dataType, "value": val})

	} else if c.Request.Method == "POST" {
		var p map[string]string
		if err := c.BindJSON(&p); err != nil {
			c.Status(400)
			return
		}
		switch dataType {
		case "string":
			rdb.Set(c, key, p["value"], 0)
		case "list":
			rdb.LPush(c, key, p["value"])
		case "hash":
			rdb.HSet(c, key, p["field"], p["value"])
		}
		c.Status(201)

	} else if c.Request.Method == "DELETE" {
		switch dataType {
		case "list":
			rdb.LRem(c, key, 1, c.Query("value"))
		case "hash":
			rdb.HDel(c, key, c.Query("field"))
		}
		c.Status(200)
	}
}

func redisKeyHandler(c *gin.Context) {
	if rdb == nil {
		c.String(503, "Redis not connected")
		return
	}
	rdb.Del(c, c.Query("key"))
	c.Status(200)
}

func redisInfoHandler(c *gin.Context) {
	if rdb == nil {
		c.String(503, "Redis not connected")
		return
	}
	info, _ := rdb.Info(c, "all").Result()
	lines := strings.Split(info, "\r\n")
	metrics := make(map[string]string)
	for _, line := range lines {
		if strings.Contains(line, ":") {
			p := strings.SplitN(line, ":", 2)
			metrics[p[0]] = p[1]
		}
	}
	c.JSON(200, metrics)
}

// --- MySQL Handlers ---
func getDB(c *gin.Context) (*sql.DB, bool) {
	// Gin 路由: /api/baseservices/mysql/metrics/:db
	dbName := strings.TrimPrefix(c.Param("db"), "/")

	db, ok := dbConnections[dbName]
	if !ok || db == nil {
		c.String(503, "DB not found")
		return nil, false
	}
	return db, true
}

func apiMetrics(c *gin.Context) {
	db, ok := getDB(c)
	if !ok {
		return
	}
	var k string
	var th, maxC, openT, slowQ int
	var q int64
	var up int64
	var bufT, bufU int
	_ = db.QueryRow("SHOW GLOBAL STATUS LIKE 'Threads_connected'").Scan(&k, &th)
	_ = db.QueryRow("SHOW GLOBAL STATUS LIKE 'Questions'").Scan(&k, &q)
	_ = db.QueryRow("SHOW GLOBAL STATUS LIKE 'Uptime'").Scan(&k, &up)
	_ = db.QueryRow("SHOW GLOBAL STATUS LIKE 'Opened_tables'").Scan(&k, &openT)
	_ = db.QueryRow("SHOW GLOBAL STATUS LIKE 'Slow_queries'").Scan(&k, &slowQ)
	_ = db.QueryRow("SHOW GLOBAL STATUS LIKE 'Innodb_buffer_pool_pages_total'").Scan(&k, &bufT)
	_ = db.QueryRow("SHOW GLOBAL STATUS LIKE 'Innodb_buffer_pool_pages_data'").Scan(&k, &bufU)
	_ = db.QueryRow("SHOW VARIABLES LIKE 'max_connections'").Scan(&k, &maxC)
	now := time.Now()
	qps := 0
	qpsMutex.Lock()
	if !lastQTime.IsZero() {
		elapsed := now.Sub(lastQTime).Seconds()
		if elapsed >= 1 && q >= lastQuestions {
			qps = int(float64(q-lastQuestions) / elapsed)
		}
	}
	lastQuestions = q
	lastQTime = now
	qpsMutex.Unlock()
	uptimeStr := fmt.Sprintf("%dd %dh %dm %ds", up/86400, (up%86400)/3600, (up%3600)/60, up%60)
	c.JSON(200, []Metric{{Time: now.Unix(), Uptime: up, UptimeStr: uptimeStr, Threads: th, QPS: qps, MaxConnections: maxC, SlowQueries: slowQ, OpenTables: openT, InnoDBBuffUsed: bufU, InnoDBBuffTotal: bufT}})
}

func apiTables(c *gin.Context) {
	db, ok := getDB(c)
	if !ok {
		return
	}
	rows, err := db.Query(`SELECT t.table_name, IFNULL(t.table_rows,0), ROUND(IFNULL(t.data_length,0)/1024/1024), IFNULL(io.count_read,0) + IFNULL(io.count_write,0) FROM information_schema.tables t LEFT JOIN performance_schema.table_io_waits_summary_by_table io ON io.object_schema = t.table_schema AND io.object_name = t.table_name WHERE t.table_schema = DATABASE() ORDER BY 3 DESC LIMIT 10;`)
	if err != nil {
		c.String(500, err.Error())
		return
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	var out []TableStat
	for rows.Next() {
		var ts TableStat
		if err := rows.Scan(&ts.Name, &ts.Rows, &ts.SizeMB, &ts.Ops); err == nil {
			out = append(out, ts)
		}
	}
	c.JSON(200, out)
}

func apiProcesslist(c *gin.Context) {
	db, ok := getDB(c)
	if !ok {
		return
	}
	rows, err := db.Query("SHOW FULL PROCESSLIST")
	if err != nil {
		c.String(500, err.Error())
		return
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	var out []ProcessListRow
	for rows.Next() {
		var id, t int
		var u, h, cCmd, s string
		var d, i sql.NullString
		if err := rows.Scan(&id, &u, &h, &d, &cCmd, &t, &s, &i); err == nil {
			out = append(out, ProcessListRow{Id: id, User: u, Host: h, DB: d.String, Command: cCmd, Time: t, State: s, Info: i.String})
		}
	}
	c.JSON(200, out)
}

func apiRepl(c *gin.Context) {
	db, ok := getDB(c)
	if !ok {
		return
	}
	rows, err := db.Query("SHOW SLAVE STATUS")
	if err != nil {
		c.JSON(200, ReplicationStatus{Role: "master"})
		return
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)
	if !rows.Next() {
		c.JSON(200, ReplicationStatus{Role: "master"})
		return
	}
	cols, _ := rows.Columns()
	vals := make([]sql.NullString, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	_ = rows.Scan(ptrs...)
	m := map[string]string{}
	for i, col := range cols {
		m[col] = vals[i].String
	}
	sb := 0
	_, _ = fmt.Sscanf(m["Seconds_Behind_Master"], "%d", &sb)
	c.JSON(200, ReplicationStatus{Role: "slave", SlaveRunning: m["Slave_IO_Running"] == "Yes" && m["Slave_SQL_Running"] == "Yes", SecondsBehind: sb})
}

func executeSQL(c *gin.Context) {
	db, ok := getDB(c)
	if !ok {
		return
	}
	type Req struct {
		SQL string `json:"sql"`
	}
	var req Req
	if err := c.BindJSON(&req); err != nil {
		c.String(400, "Invalid JSON")
		return
	}
	if strings.TrimSpace(req.SQL) == "" {
		c.String(400, "Empty SQL")
		return
	}
	rows, err := db.Query(req.SQL)
	if err != nil {
		c.JSON(200, SqlResult{Error: err.Error()})
		return
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	cols, _ := rows.Columns()
	var allRows [][]string
	for rows.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		_ = rows.Scan(ptrs...)
		row := make([]string, len(cols))
		for i, v := range vals {
			if v.Valid {
				row[i] = v.String
			} else {
				row[i] = "NULL"
			}
		}
		allRows = append(allRows, row)
	}
	c.JSON(200, SqlResult{Columns: cols, Rows: allRows})
}

// --------------------------------------------------------------------------------
// 代理逻辑 (适配 Gin & 修复 MinIO 路径与WebSocket问题)
// --------------------------------------------------------------------------------
// --------------------------------------------------------------------------------
// 代理逻辑 (适配 Nginx /gogogo/ 前缀 + 调试日志)
// --------------------------------------------------------------------------------
func setupGinProxies(r *gin.Engine) {
	// Helper: 响应修改逻辑
	createRewriteFunc := func(internalBasePath string) func(*http.Response) error {
		return func(resp *http.Response) error {
			// 1. 获取前缀
			externalPrefix := resp.Request.Header.Get("X-Forwarded-Prefix")
			if externalPrefix == "" {
				referer := resp.Request.Header.Get("Referer")
				if referer != "" {
					if u, err := url.Parse(referer); err == nil {
						if idx := strings.Index(u.Path, internalBasePath); idx > 0 {
							externalPrefix = u.Path[:idx]
						}
					}
				}
			}

			finalBasePath := strings.TrimRight(externalPrefix, "/") + internalBasePath

			// 3. 清除安全限制
			resp.Header.Del("X-Frame-Options")
			resp.Header.Del("Content-Security-Policy")

			// 4. 重写 Body
			contentType := resp.Header.Get("Content-Type")
			// [安全检查] 仅对明确的文本类型进行处理，避免处理大文件或二进制流
			if strings.Contains(contentType, "text/html") ||
				strings.Contains(contentType, "application/javascript") ||
				strings.Contains(contentType, "text/css") ||
				strings.Contains(contentType, "application/json") ||
				strings.Contains(contentType, "text/plain") {

				// [增强] 处理 GZIP
				var reader io.ReadCloser
				var err error
				isGzip := resp.Header.Get("Content-Encoding") == "gzip"

				if isGzip {
					reader, err = gzip.NewReader(resp.Body)
					if err != nil {
						// 如果解压失败，不要报错，而是直接返回原始内容（不做替换）
						// 这样至少用户能看到点东西，而不是 502
						// 此时 resp.Body 还没被读取，所以直接返回 nil 即可
						return nil
					}
				} else {
					reader = resp.Body
				}

				// 读取内容
				bodyBytes, err := io.ReadAll(reader)
				// 无论如何，关闭 reader
				_ = reader.Close()

				if err != nil {
					// 读取失败，同样放弃修改，直接返回
					return nil
				}

				// 如果是 Gzip，我们已经解压了，所以要删除这个头
				if isGzip {
					resp.Header.Del("Content-Encoding")
				}

				bodyString := string(bodyBytes)

				// === 核心替换逻辑 ===
				bodyString = strings.ReplaceAll(bodyString, `src="/`, `src="`+finalBasePath)
				bodyString = strings.ReplaceAll(bodyString, `href="/`, `href="`+finalBasePath)
				bodyString = strings.ReplaceAll(bodyString, `action="/`, `action="`+finalBasePath)
				bodyString = strings.ReplaceAll(bodyString, `src='/`, `src='`+finalBasePath)
				bodyString = strings.ReplaceAll(bodyString, `href='/`, `href='`+finalBasePath)
				bodyString = strings.ReplaceAll(bodyString, `"/api/v1/`, `"`+finalBasePath+`api/v1/`)
				bodyString = strings.ReplaceAll(bodyString, `'ws://'`, `'ws://'+window.location.host+'`+finalBasePath)
				bodyString = strings.ReplaceAll(bodyString, `'wss://'`, `'wss://'+window.location.host+'`+finalBasePath)

				buf := bytes.NewBufferString(bodyString)
				resp.Body = io.NopCloser(buf)
				resp.ContentLength = int64(buf.Len())
				resp.Header.Set("Content-Length", strconv.Itoa(buf.Len()))
			}
			return nil
		}
	}

	// --- RabbitMQ Proxy ---
	if appConfig.RabbitMQAdminPort > 0 {
		rabbitURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", appConfig.RabbitMQAdminPort))
		proxy := httputil.NewSingleHostReverseProxy(rabbitURL)
		proxy.ModifyResponse = createRewriteFunc("/api/baseservices/rabbitmq/")

		proxy.Director = func(req *http.Request) {
			req.URL.Scheme = rabbitURL.Scheme
			req.URL.Host = rabbitURL.Host
			req.URL.Path = strings.TrimPrefix(req.URL.Path, "/api/baseservices/rabbitmq")
			req.URL.RawPath = ""
			req.Host = rabbitURL.Host
			if req.Header.Get("Origin") != "" {
				req.Header.Set("Origin", fmt.Sprintf("%s://%s", rabbitURL.Scheme, rabbitURL.Host))
			}
			// [关键] 告诉上游我们不接受压缩
			req.Header.Del("Accept-Encoding")
		}
		r.Any("/api/baseservices/rabbitmq/*path", func(c *gin.Context) {
			if c.Param("path") == "/" && !strings.HasSuffix(c.Request.URL.Path, "/") {
				prefix := c.GetHeader("X-Forwarded-Prefix")
				if prefix == "" {
					referer := c.GetHeader("Referer")
					if referer != "" {
						if u, err := url.Parse(referer); err == nil {
							internalBasePath := "/api/baseservices/rabbitmq/"
							if idx := strings.Index(u.Path, internalBasePath); idx > 0 {
								prefix = u.Path[:idx]
							}
						}
					}
				}
				c.Redirect(http.StatusMovedPermanently, prefix+"/api/baseservices/rabbitmq/")
				return
			}
			proxy.ServeHTTP(c.Writer, c.Request)
		})
	}

	// --- MinIO Proxy (Target Console Port 9001) ---
	targetMinio := "http://127.0.0.1:9001"
	if appConfig.MinioURL != "" && !strings.Contains(appConfig.MinioURL, ":9000") {
		targetMinio = appConfig.MinioURL
	}

	minioURL, err := url.Parse(targetMinio)
	if err == nil {
		minioProxy := httputil.NewSingleHostReverseProxy(minioURL)
		minioProxy.ModifyResponse = createRewriteFunc("/api/baseservices/minio/")

		minioProxy.Director = func(req *http.Request) {
			req.URL.Scheme = minioURL.Scheme
			req.URL.Host = minioURL.Host
			req.Host = minioURL.Host
			targetOrigin := fmt.Sprintf("%s://%s", minioURL.Scheme, minioURL.Host)
			req.Header.Set("Origin", targetOrigin)
			path := req.URL.Path
			if strings.HasPrefix(path, "/api/baseservices/minio") {
				path = strings.TrimPrefix(path, "/api/baseservices/minio")
			}
			if path == "" {
				path = "/"
			}
			req.URL.Path = path
			req.URL.RawPath = ""
			if req.Header.Get("Connection") == "Upgrade" {
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Upgrade", "websocket")
			}
			// [关键] 告诉上游我们不接受压缩
			req.Header.Del("Accept-Encoding")
		}

		r.Any("/api/baseservices/minio/*path", func(c *gin.Context) {
			path := c.Param("path")
			if (path == "" || path == "/") && !strings.HasSuffix(c.Request.URL.Path, "/") {
				prefix := c.GetHeader("X-Forwarded-Prefix")
				if prefix == "" {
					referer := c.GetHeader("Referer")
					if referer != "" {
						if u, err := url.Parse(referer); err == nil {
							internalBasePath := "/api/baseservices/minio/"
							if idx := strings.Index(u.Path, internalBasePath); idx > 0 {
								prefix = u.Path[:idx]
							}
						}
					}
				}
				c.Redirect(http.StatusMovedPermanently, prefix+"/api/baseservices/minio/")
				return
			}
			minioProxy.ServeHTTP(c.Writer, c.Request)
		})
	}
}
