# UEM Deployment Tools

> 一个用于UEM（统一端点管理）部署、监控和维护的辅助工具集。

该工具包含一个在 Windows 上运行的客户端 (`uemtools.exe`) 和一个在目标 Linux 服务器上运行的代理 (`cncyagent`)。用户通过客户端与远程服务器上的代理进行交互，实现各种自动化操作和监控。

## 🚀 产品功能 (Product Features)

工具的前端界面提供了丰富的功能，主要分为以下几个模块：

#### 1. 🔍 操作系统
- **实时资源监控**: 以图表形式实时展示 CPU、内存使用率、系统平均负载和网络流量 (Rx/Tx)。
- **基础环境检测**: 一键检查 CPU 核心数、内存大小、系统架构、操作系统版本和 `ulimit` 限制，并给出是否满足建议规格的判断。
- **磁盘空间概览**: 以进度条形式清晰展示所有挂载点的磁盘使用情况。
- **安全与网络**:
    - 检查并提供一键修复 SELinux 和防火墙状态的功能。
    - 检查并提供一键开启 SSH 隧道转发 (`AllowTcpForwarding`) 的功能。
    - **网络端口**: 实时显示 `netstat -nltp` 的结果，方便查看端口监听状态。
    - **TCP 连接数**: 实时统计当前系统的总 TCP 连接数量。
- **UEM 服务监控**: 自动检测 UEM 是否安装，并实时展示所有相关服务的运行状态，提供单独重启服务的功能。
- **MinIO 检测**: 检查 MinIO 存储桶是否存在以及其访问策略是否为公开，并提供一键修复为公开读策略的功能。

#### 2. 🔧 环境依赖
- **ISO 挂载**: 支持通过上传 ISO 镜像文件或指定服务器上的本地路径，自动将其挂载并配置为本地 YUM 源。
- **RPM 安装**: 支持在线上传 RPM 包并直接在服务器上进行安装。

#### 3. 📦 部署/更新
- **灵活的部署路径**: 支持指定任意服务器目录作为工作目录。
- **脚本自动检测**: 自动检测指定目录下是否存在 `install.sh` (用于首次部署) 和 `mdm.sh` (用于更新)，并根据检测结果动态启用相关操作按钮。
- **一键式操作**:
    - **首次部署**: 执行 `install.sh`。
    - **UEM 更新**: 执行 `mdm.sh uem`。
    - **WebUI 更新**: 执行 `mdm.sh webui`。
    - **Tomcat 更新**: 执行 `mdm.sh tomcat`。
- **实时输出**: 所有部署和更新操作的输出都会实时显示在网页终端中。

#### 4. 📂 文件管理
- 提供一个简单的网页版文件浏览器。
- 支持在服务器上浏览目录、返回上级。
- 支持从本地上传文件到服务器的当前目录。
- 支持下载服务器上的文件。

#### 5. 💻 终端
- 提供一个功能完整的、交互式的网页 Shell 终端，可以直接操作服务器。

#### 6. 📜 日志查看
- **多日志源**: 预设了 Tomcat, Nginx, AppServer 等多个常用服务的日志。
- **实时流式传输**: 点击即可实时查看日志 (`tail -f`)。
- **便捷操作**: 支持自动滚动、清空显示和一键下载日志文件。

#### 7. ⚙️ 基础服务
- **零配置监控**: 工具能够**自动读取** UEM 的核心配置文件 `global.properties`，从中获取 Redis、MySQL、RabbitMQ 等服务的连接信息，无需用户手动输入。
- **Redis**:
    - 实时查看 Redis 的各项性能指标。
    - 浏览、查看、修改和删除 Key-Value。
- **MySQL**:
    - 实时监控多实例 (mdm, multitenant) 的 QPS、线程数、连接数、主从延迟等状态。
    - 以图表形式展示表空间和表操作的 Top 10。
    - 查看实时进程列表 (`SHOW FULL PROCESSLIST`)。
    - 提供 SQL 执行器，可直接执行查询或修改语句并查看结果。
- **RabbitMQ & MinIO**:
    - 通过反向代理将它们的管理后台无缝内嵌到工具中，无需再次登录或暴露端口。

## 🛠️ 技术实现 (Technical Implementation)

#### 1. 项目架构
本项目采用 **C/S 架构**，并通过 **Wails** 技术将客户端和服务端能力打包成一个独立的 Windows 桌面应用。

- **Agent (后端代理)**: 一个纯 Go 语言编写的轻量级 Web 服务器，运行在目标 Linux 服务器上。它不依赖任何外部运行时，具有出色的跨平台和性能表现。
- **Client (前端客户端)**: 一个基于 **Wails v2** 的 Windows 桌面应用。Wails 使用 Go 作为后端，通过 WebView2 渲染前端 UI，实现了接近本机的性能和体验。

---

#### 2. Agent 核心实现方法

Agent 是所有远程操作的执行者。

- **Web 服务**:
    - 使用 **Gin** (`github.com/gin-gonic/gin`) 框架构建，它以高性能和简洁的 API 设计著称。
    - Agent 启动后，会监听一个端口（默认为 `9898`），提供两类服务：
        1.  **RESTful API**: 用于处理无状态、一次性的请求，如获取系统信息 (`/api/check`)、列出文件 (`/api/fs/list`) 等。
        2.  **WebSocket 服务**: 用于需要持久连接和实时双向通信的场景。

- **系统信息与服务发现**:
    - **命令执行**: 大部分系统信息通过 `os/exec` 包执行标准的 Linux 命令并解析其 `stdout` 来获得。例如：
        - `netstat -nltp`: 获取监听中的 TCP 端口信息。Go 代码会逐行解析输出，提取协议、地址、PID 等关键字段，并将其构造成 JSON 数组返回给前端。
        - `df -h`: 获取磁盘分区信息。
        - `ulimit -n`: 获取文件句柄数限制。
    - **文件读取**: 部分核心指标（如内存、CPU）直接读取 `/proc` 文件系统下的文件（如 `/proc/meminfo`, `/proc/loadavg`）来获取原始数据，这样做比执行命令更高效。
    - **自动服务发现 (通过 `global.properties`)**:
        - **定位文件**: Agent 启动时，会自动尝试读取 UEM 的核心配置文件 `/opt/emm/current/config/global.properties`。
        - **解析配置**: 使用 `github.com/magiconair/properties` 库解析该 `.properties` 文件。
        - **提取关键信息**: 从文件中读取以下关键配置项：
            - `system.redis.host`, `system.redis.port`, `system.redis.password`
            - `jdbc.url`, `jdbc.username`, `jdbc.password` (用于主数据库 `mdm`)
            - `jdbc.multitenant.url`, `jdbc.multitenant.username`, `jdbc.multitenant.password` (用于多租户数据库)
            - `spring.rabbitmq.addresses`, `rabbitmq.admin.port`
            - `storage.minio.url`
        - **建立连接**: 基于这些提取出的信息，Agent 初始化到各个服务的连接池（如 `database/sql` 和 `go-redis`），从而实现了对这些基础服务的“零配置”监控和管理。

- **交互式终端与实时日志**:
    - **伪终端 (PTY)**: 使用 `github.com/creack/pty` 库在服务器上创建一个伪终端，并将用户的 Shell（如 `/bin/bash`）附加到该终端上。
    - **WebSocket 桥接**:
        1.  前端（使用 **Xterm.js**）捕获用户的按键输入，通过 WebSocket 将数据发送到 Agent。
        2.  Agent 接收到数据后，将其写入 PTY 的 master 端，模拟真实终端的输入。
        3.  Shell 在 PTY 的 slave 端执行命令，其输出被 Agent 从 PTY 的 master 端读取。
        4.  Agent 将读取到的输出通过 WebSocket 实时地回传给前端的 Xterm.js 进行显示。
    - **实时日志** (`tail -f`) 的实现与此类似，只是将 `bash` 换成了 `tail -f /path/to/log` 命令。

---

#### 3. Client 与 Agent 的通信流程

这是整个工具链的核心。

1.  **启动与连接**:
    - 用户在 Wails 应用中输入服务器的 IP、用户名和密码。
    - Wails 的 Go 后端使用 `golang.org/x/crypto/ssh` 包建立一个到目标服务器的 SSH 连接。

2.  **Agent 部署与启动**:
    - 项目在编译时，已通过 Go 的交叉编译功能将 Agent 代码编译成 `amd64` 和 `arm64` 两种架构的 Linux 二进制文件，并将其**嵌入**到 Wails 客户端 `uemtools.exe` 中。
    - SSH 连接成功后，客户端首先检测服务器的架构 (`uname -m`)。
    - 根据架构，选择对应的 Agent 二进制文件，通过 SSH 的 SFTP 功能将其上传到服务器的一个临时目录（如 `/tmp/cncyagent_amd64`）。
    - 客户端通过 SSH 执行命令，为 Agent 添加执行权限 (`chmod +x`)，然后启动它。

3.  **建立通信隧道 (SSH 端口转发)**:
    - Agent 在服务器上监听的是 `127.0.0.1:9898`，这是一个仅限服务器本地访问的地址，保证了安全性。
    - 为了让 Wails 客户端（运行在用户的 Windows 电脑上）能够访问到这个端口，客户端在建立 SSH 连接的同时，会设置一个**本地端口转发** (Local Port Forwarding)。
    - 这意味着，客户端会监听自己电脑上的一个端口（例如 `localhost:39898`），并将所有发送到该端口的流量通过 SSH 安全隧道转发到服务器的 `127.0.0.1:9898`。

4.  **前端交互**:
    - Wails 应用的前端 UI 是一个内嵌的网页。这个网页中的所有 API 请求（通过 `fetch`）和 WebSocket 连接，实际上访问的都是 `http://localhost:39898`。
    - 这些请求被 SSH 客户端拦截，加密后通过隧道发送到服务器，最终到达 Agent。Agent 处理后返回的数据再沿着隧道传回前端。
    - 对用户和前端代码来说，整个过程是透明的，就像在直接访问一个本地服务。

---

#### 4. 反向代理的实现

为了将 RabbitMQ 和 MinIO 的 Web 管理后台无缝集成，Agent 内置了一个基于 `net/http/httputil.NewSingleHostReverseProxy` 的反向代理。

- **解决跨域和端口问题**: 无需将 RabbitMQ/MinIO 的管理端口暴露到公网，前端只需与 Agent 的端口通信。
- **路径重写**: 代理会拦截特定路径的请求（如 `/api/baseservices/rabbitmq/*`），将请求转发到内部的管理端口（如 `127.0.0.1:15672`），并将响应返回给前端。
- **HTML 内容修复**: 由于这些管理后台的 HTML/JS/CSS 文件中的资源路径通常是绝对路径（如 `/css/main.css`），直接在 iframe 中加载会导致 404。代理会在返回响应前，读取 HTML/JS 内容，通过字符串替换，将这些路径动态修改为带代理前缀的路径（如 `src="/css/main.css"` -> `src="/api/baseservices/rabbitmq/css/main.css"`），从而确保所有资源都能被正确加载。

## 🏗️ 如何构建 (How to Build)

项目提供了一个 `build.sh` 脚本来自动化整个构建流程。

1.  确保您的开发环境中已安装 Go 和 Wails v2。
2.  在项目根目录下，执行以下命令：
    ```sh
    sh build.sh
    ```
3.  脚本会完成以下工作：
    - 交叉编译生成两个 Linux Agent: `cncyagent_amd64` 和 `cncyagent_arm64`。
    - 使用 Wails 构建 Windows 客户端 `uemtools.exe`，并将上述 Agent 二进制文件嵌入其中。
    - 将所有生成物移动到 `build/bin/` 目录下。

构建成功后，所有产物都位于 `build/bin/` 目录中。

## Nginx 配置示例 (Nginx Configuration Example)

如果您希望通过 Nginx 反向代理来访问 Agent，可以使用以下配置：

```nginx
location /gogogo/ {
    # 注意端口号改成你的 agent 端口，末尾的 / 用于去除 /gogogo 前缀
    proxy_pass http://127.0.0.1:9898/;
   
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Prefix /gogogo;
    # 支持 WebSocket
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
}
```
