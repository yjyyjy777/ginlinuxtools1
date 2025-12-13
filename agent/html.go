package main

// 修复说明：
// 1. 之前代码中的 \\n 导致 JS 解析为字面量字符 "\n" 而非换行符，致使 Markdown 渲染时所有内容挤在一行。
//    现已全部替换为 \n，确保 JS 字符串中包含真正的换行符。
// 2. 保留了 \x60 用于在 JS 字符串中表示反引号，避免与 Go 的 Raw String 冲突。
// 3. [路径修复] 移除了硬编码的 API_BASE 和 UPLOAD_URL 常量，所有 fetch 和 XHR 请求都使用相对路径，以同时支持根路径和子路径部署。

const htmlPage = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <title>UEM Deployment Tools</title>
    <script>
        // 强制尾部斜杠，防止相对路径资源加载错误
        if (!window.location.pathname.endsWith('/') && !window.location.pathname.endsWith('.html')) {
            var newUrl = window.location.protocol + "//" + window.location.host + window.location.pathname + "/" + window.location.search;
            window.history.replaceState(null, null, newUrl);
            window.location.reload();
         }
    </script>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/xterm@5.3.0/css/xterm.min.css" />
    <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.4.0/css/all.min.css">
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>
    <style>
        body { font-family: 'Segoe UI', sans-serif; background: #2c3e50; margin: 0; height: 100vh; display: flex; flex-direction: column; overflow: hidden; }
        .navbar { background: #34495e; padding: 0 20px; height: 50px; display: flex; align-items: center; border-bottom: 1px solid #1abc9c; flex-shrink: 0; }
        .brand { color: #fff; font-weight: bold; font-size: 18px; margin-right: 20px; }
        .tab-btn { background: transparent; border: none; color: #bdc3c7; font-size: 13px; padding: 0 10px; height: 100%; cursor: pointer; transition: 0.3s; border-bottom: 3px solid transparent; }
        .tab-btn:hover { color: white; background: rgba(255,255,255,0.05); }
        .tab-btn.active { color: #1abc9c; border-bottom: 3px solid #1abc9c; background: rgba(26, 188, 156, 0.1); }
        .content { flex: 1; position: relative; background: #ecf0f1; overflow: hidden; display: flex; flex-direction: column; }
        .panel { display: none; width: 100%; height: 100%; padding: 20px; box-sizing: border-box; overflow-y: auto; }
        .panel.active { display: block; }
        #panel-baseservices { padding: 0; display: none; flex-direction: column; height: 100%; overflow: hidden; }
        #panel-baseservices.active { display: flex; }
        .container-box { padding: 20px; max-width: 1200px; margin: 0 auto; width: 100%; box-sizing: border-box; }
        .card { background: white; padding: 15px; border-radius: 6px; box-shadow: 0 2px 5px rgba(0,0,0,0.1); margin-bottom: 15px; display: flex; flex-direction: column; }
        h3 { margin-top: 0; border-bottom: 2px solid #eee; padding-bottom: 10px; color: #2c3e50; display: flex; justify-content: space-between; align-items: center; font-size: 16px; }
        .term-box { flex: 1; background: #1e1e1e; padding: 10px; overflow-y: auto; border-radius: 6px; color: #0f0; font-family: Consolas, monospace; font-size: 13px; white-space: pre-wrap; border: 1px solid #333; }
        .full-term { width: 100%; height: 100%; background: #000; padding: 10px; box-sizing: border-box; }
        table { width: 100%; border-collapse: collapse; margin-top: 10px; font-size: 14px; }
        th, td { text-align: left; padding: 8px; border-bottom: 1px solid #eee; }
        th { background-color: #f8f9fa; color: #666; position: sticky; top: 0; }
        .pass { color: #27ae60; font-weight: bold; }
        .fail { color: #c0392b; font-weight: bold; }
        .warn { color: #f39c12; font-weight: bold; }
        .progress-bg { width: 100%; background-color: #e0e0e0; border-radius: 4px; height: 16px; overflow: hidden; position: relative; }
        .progress-bar { height: 100%; text-align: center; line-height: 16px; color: white; font-size: 10px; transition: width 0.5s; }
        .bg-green { background-color: #27ae60; } .bg-orange { background-color: #f39c12; } .bg-red { background-color: #c0392b; }
        .disk-text { font-size: 12px; color: #666; margin-top: 2px; display: flex; justify-content: space-between; }
        .fm-toolbar { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; padding-bottom: 10px; border-bottom: 1px solid #eee; }
        .fm-path { flex: 1; padding: 5px; border: 1px solid #ddd; border-radius: 4px; background: #f9f9f9; font-family: monospace; }
        .fm-list { flex: 1; overflow-y: auto; }
        .icon-dir { color: #f39c12; margin-right: 5px; } .icon-file { color: #95a5a6; margin-right: 5px; }
        .link-dir { color: #2980b9; cursor: pointer; text-decoration: none; font-weight: bold; } .link-dir:hover { text-decoration: underline; }
        .log-layout { display: flex; height: 100%; border: 1px solid #ddd; border-radius: 6px; overflow: hidden; background: white; }
        .log-sidebar { width: 240px; background: #f8f9fa; border-right: 1px solid #ddd; display: flex; flex-direction: column; }
        .log-sidebar-header { padding: 10px; background: #e9ecef; font-weight: bold; font-size: 14px; border-bottom: 1px solid #ddd; }
        .log-list { flex: 1; overflow-y: auto; list-style: none; padding: 0; margin: 0; }
        .log-item { padding: 8px 12px; cursor: pointer; font-size: 13px; color: #333; border-bottom: 1px solid #f1f1f1; transition: 0.2s; display: flex; justify-content: space-between; align-items: center; }
        .log-item:hover { background: #e2e6ea; } .log-item.active { background: #3498db; color: white; border-left: 4px solid #2980b9; }
        .log-viewer-container { flex: 1; display: flex; flex-direction: column; background: #1e1e1e; }
        .log-viewer-header { padding: 5px 10px; background: #2c3e50; color: #ecf0f1; font-size: 12px; display: flex; justify-content: space-between; align-items: center; }
        .log-content { flex: 1; overflow-y: auto; padding: 10px; font-family: 'Consolas', monospace; font-size: 12px; color: #dcdcdc; white-space: pre-wrap; word-break: break-all; }
        button { background: #2980b9; color: white; border: none; padding: 6px 12px; border-radius: 4px; cursor: pointer; font-size: 13px; transition: 0.2s; }
        button:hover { background: #3498db; } button:disabled { background: #95a5a6; cursor: not-allowed; opacity: 0.6; }
        .btn-sm { padding: 4px 8px; font-size: 12px; }
        .btn-fix { background: #e67e22; } .btn-fix:hover { background: #d35400; }
        .btn-green { background: #27ae60; } .btn-green:hover { background: #219150; }
        .btn-orange { background: #e67e22; } .btn-orange:hover { background: #d35400; }
        .btn-red { background: #e74c3c; } .btn-red:hover { background: #c0392b; }
        .btn-restart { background: #e74c3c; } .btn-restart:hover { background: #c0392b; }
        .btn-dl-log { background: transparent; border: 1px solid #ccc; color: #666; padding: 2px 6px; border-radius: 3px; font-size: 11px; cursor: pointer; }
        .btn-dl-log:hover { background: #27ae60; color: white; border-color: #27ae60; }
        input[type="file"], input[type="text"], textarea, select { border: 1px solid #ccc; padding: 5px; background: white; font-size: 13px; border-radius: 4px; }
        .grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }
        .grid-4 { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; }
        .about-table td { padding: 10px; }
        .about-table tr:not(:last-child) td { border-bottom: 1px solid #f0f0f0; }
        .bs-header { padding: 10px 20px; background: #e9ecef; display: flex; gap: 5px; border-bottom: 1px solid #ddd; flex-shrink: 0; }
        .sub-tab-btn { background: #fff; color: #666; border: 1px solid #ddd; padding: 6px 14px; cursor: pointer; border-radius: 4px; font-size: 13px; }
        .sub-tab-btn:hover { background: #f8f9fa; }
        .sub-tab-btn.active { background: #2980b9; color: white; border-color: #2980b9; }
        .sub-panel { display: none; flex: 1; flex-direction: column; overflow: hidden; background: #fff; width: 100%; height: 100%; }
        .sub-panel.active { display: flex; }
        .modal-backdrop { position: fixed; top: 0; left: 0; width: 100%; height: 100%; background-color: rgba(0,0,0,0.5); z-index: 100; display: none; }
        .modal { position: fixed; top: 50%; left: 50%; transform: translate(-50%, -50%); background-color: #fff; padding: 25px; border-radius: 8px; box-shadow: 0 5px 15px rgba(0,0,0,0.3); z-index: 101; width: 90%; max-width: 700px; display: none; max-height: 80vh; overflow-y: auto; }
        .modal-header { display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid #dee2e6; padding-bottom: 10px; margin-bottom: 20px; }
        .modal-title { margin: 0; font-size: 1.25rem; }
        .modal-close { background: none; border: none; font-size: 1.5rem; cursor: pointer; }
        .modal-body { margin-bottom: 20px; }
        .modal-footer { border-top: 1px solid #dee2e6; padding-top: 15px; margin-top: 20px; text-align: right; }
        .list-item, .hash-item { display: flex; justify-content: space-between; align-items: center; padding: 8px; border-bottom: 1px solid #e9ecef; }
        .iframe-container { flex: 1; width: 100%; height: 100%; border: none; display: block; }
        .sql-table-container { overflow: auto; max-height: 400px; border: 1px solid #ddd; margin-top: 10px; }
        .sql-table { width: 100%; border-collapse: collapse; font-size: 13px; font-family: Consolas, monospace; white-space: nowrap; }
        .sql-table th { background: #f8f9fa; position: sticky; top: 0; border-bottom: 2px solid #ddd; padding: 8px; text-align: left; color: #333; }
        .sql-table td { border-bottom: 1px solid #eee; padding: 6px 8px; color: #444; }
        .sql-table tr:hover { background-color: #f1f1f1; }
        .redis-key-cell { max-width: 450px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
        #panel-help .container-box { max-width: 900px; }
        #panel-help h1, #panel-help h2, #panel-help h3, #panel-help h4 { border-bottom: 1px solid #ddd; padding-bottom: 8px; margin-top: 24px; margin-bottom: 16px; }
        #panel-help code { background-color: #f1f1f1; padding: 2px 5px; border-radius: 4px; font-family: Consolas, monospace; }
        #panel-help pre { background-color: #2d2d2d; color: #f1f1f1; padding: 15px; border-radius: 6px; overflow-x: auto; }
        #panel-help pre code { background: none; padding: 0; }
        #panel-help ul, #panel-help ol { line-height: 1.6; }
        #panel-help blockquote { border-left: 4px solid #ccc; padding-left: 15px; color: #666; margin-left: 0; }
        
        /* [ADDED] Progress Bar Styles */
        #uploadProgressContainer {
            display: none; /* Initially hidden */
            width: 100%;
            background-color: #e0e0e0;
            border-radius: 4px;
            margin-top: 10px;
            height: 20px;
            overflow: hidden;
        }
        #uploadProgressBar {
            width: 0%;
            height: 100%;
            background-color: #27ae60;
            text-align: center;
            line-height: 20px;
            color: white;
            font-size: 12px;
            transition: width 0.3s ease;
        }
    </style>
</head>
<body>
<div class="navbar">
    <button class="tab-btn active" onclick="switchTab('check')">🔍 操作系统</button>
    <button class="tab-btn" onclick="switchTab('deps')">🔧 环境依赖</button>
    <button class="tab-btn" onclick="switchTab('deploy')">📦 部署/更新</button>
    <button class="tab-btn" onclick="switchTab('files')">📂 文件管理</button>
    <button class="tab-btn" onclick="switchTab('terminal')">💻 终端</button>
    <button class="tab-btn" onclick="switchTab('logs')">📜 日志查看</button>
    <button class="tab-btn" onclick="switchTab('baseservices')">⚙️ 基础服务</button>
    <button class="tab-btn" onclick="switchTab('about')">ℹ️ 关于</button>
    <button class="tab-btn" onclick="switchTab('help')" style="margin-left: auto;">❓ 帮助</button>
</div>
<div class="content">
    <div id="panel-check" class="panel active">
        <div class="grid-2">
            <div class="card">
                <h3>📈 系统资源 (3秒刷新)</h3>
                <div style="height: 200px; position: relative;">
                    <canvas id="sysChart"></canvas>
                </div>
            </div>
            <div class="card">
                <h3>🌐 网络流量 (KB/s)</h3>
                <div style="height: 200px; position: relative;">
                    <canvas id="netChart"></canvas>
                </div>
            </div>
        </div>
        <br>
        <div class="grid-2">
            <div>
                <div class="card"><h3>🖥️ 基础环境 <button onclick="runCheck()" class="btn-sm"><i class="fas fa-sync"></i> 刷新</button></h3><table id="baseTable"><tbody><tr><td>加载中...</td></tr></tbody></table></div>
                <div class="card"><h3>💾 磁盘空间概览</h3><div id="diskList" style="margin-top:10px;">加载中...</div></div>
                <div class="card"><h3>🛡️ 安全与网络</h3><table id="secTable"><tbody><tr><td>加载中...</td></tr></tbody></table></div>
            </div>
            <div>
                <div class="card"><h3>🚀 UEM 服务监控</h3><div id="uemStatusBox"><p>检测 UEM 安装状态...</p></div></div>
                <div class="card"><h3>🗄️ MinIO 检测</h3><table id="minioTable"><tbody><tr><td>加载中...</td></tr></tbody></table></div>
            </div>
        </div>
        <div class="grid-2">
            <div class="card">
                <h3>🔌 网络端口 (netstat -nltp)</h3>
                <div style="height: 200px; overflow-y: auto;">
                    <table id="netstatTable">
                        <thead>
                            <tr>
                                <th>协议</th>
                                <th>本地地址</th>
                                <th>PID/程序名</th>
                            </tr>
                        </thead>
                        <tbody id="netstatTableBody">
                            <tr><td colspan="3">加载中...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>
            <div class="card">
                <h3>🔗 TCP 连接数</h3>
                <div id="tcpConnCountBox" style="font-size: 2em; font-weight: bold; text-align: center; padding: 20px;">加载中...</div>
            </div>
        </div>
    </div>
    
    <div id="panel-deps" class="panel"><div class="container-box" style="max-width: 1000px;"><div class="card"><h3>💿 ISO 挂载 (配置本地 YUM)</h3><div style="display:flex; flex-direction:column; gap:10px;"><div style="display:flex; align-items:center; gap:10px;"><span style="width:80px; color:#666;">上传镜像:</span><input type="file" id="isoInput" accept=".iso" style="width:300px;"><button onclick="mountIso()">上传并挂载</button></div><div style="display:flex; align-items:center; gap:10px;"><span style="width:80px; color:#666;">本地路径:</span><input type="text" id="isoPathInput" placeholder="/tmp/kylin.iso" style="width:300px;"><button class="btn-orange" onclick="mountLocalIso()">使用本地文件</button></div></div><div id="yum-log" class="term-box" style="height:120px;margin-top:10px">等待操作...</div></div><div class="card"><h3>🛠️ RPM 安装</h3><div style="display:flex;gap:10px"><input type="file" id="rpmInput" accept=".rpm"><button onclick="installRpm()">执行安装</button></div><div id="rpm-log" class="term-box" style="height:120px;margin-top:10px"></div></div></div></div>
    
    <!-- 部署与更新面板 (重构版) -->
    <div id="panel-deploy" class="panel">
        <div class="container-box" style="max-width: 1000px;">
            
            <!-- 1. 目录设置 -->
            <div class="card">
                <h3>📂 1. 设置工作目录</h3>
                <div style="display:flex; gap:10px; align-items:center;">
                    <span style="color:#666; font-size:13px;">路径:</span>
                    <input type="text" id="manualPathInput" placeholder="/root/install-cncy" value="/root/install-cncy" style="flex:1; font-family:monospace;">
                    <button class="btn-orange" onclick="checkManualPath()">检测脚本</button>
                </div>
                <div id="pathCheckMsg" style="margin-top:5px; font-size:12px; height:18px;"></div>
            </div>

            <!-- 2. 文件上传 -->
            <div class="card">
                <h3>📤 2. 上传更新包 (上传后自动解压)</h3>
                <div style="background:#f8f9fa; padding:10px; border-radius:4px; font-size:12px; color:#666; margin-bottom:10px; line-height: 1.6;">
                    <strong>请根据更新类型上传对应文件：</strong><br>
                    1. 更新 WebUI &nbsp;&nbsp;➔ 上传 <code>WebUI.tar.gz</code><br>
                    2. 更新 Tomcat ➔ 上传 <code>apache-tomcat-*.zip</code><br>
                    3. 全量更新 UEM ➔ 上传 <code>UEM-*.tar.gz</code>
                </div>
                
                <!-- [MODIFIED] New Upload UI -->
                <div style="display:flex; flex-direction:column; gap:10px;">
                    <div style="display:flex; align-items:center; gap:10px;">
                        <span style="color:#666; font-size:13px; width:80px;">上传路径:</span>
                        <input type="text" id="uploadPathInput" value="/root" style="flex:1; font-family:monospace;">
                    </div>
                    <div style="display:flex; align-items:center; gap:10px;">
                        <span style="color:#666; font-size:13px; width:80px;">选择文件:</span>
                        <input type="file" id="fileInput" style="flex:1;">
                        <button id="uploadButton" onclick="uploadFileWithProgress()">上传</button>
                    </div>
                    <div id="uploadProgressContainer">
                        <div id="uploadProgressBar"></div>
                    </div>
                    <div id="uploadStatus" style="font-weight:bold; font-size:12px; height: 16px;"></div>
                </div>

            </div>

            <!-- 3. 执行操作 -->
            <div class="card" style="flex:1">
                <h3>🚀 3. 执行操作</h3>
                
                <div style="display:grid; grid-template-columns: repeat(4, 1fr); gap:10px; margin-bottom:15px;">
                    <!-- 首次部署 -->
                    <button id="btnInstall" class="btn-green" onclick="startScript('install')" disabled>
                        <i class="fas fa-play"></i> 首次部署<br><span style="font-size:10px; opacity:0.8">(install.sh)</span>
                    </button>
                    
                    <!-- UEM 全量更新 -->
                    <button id="btnUEM" class="btn-red" onclick="startScript('update', 'uem')" disabled>
                        <i class="fas fa-sync"></i> 更新 UEM<br><span style="font-size:10px; opacity:0.8">(mdm.sh uem)</span>
                    </button>
                    
                    <!-- WebUI 独立更新 -->
                    <button id="btnWebUI" class="btn-orange" onclick="startScript('update', 'webui')" disabled>
                        <i class="fas fa-columns"></i> 更新 WebUI<br><span style="font-size:10px; opacity:0.8">(mdm.sh webui)</span>
                    </button>
                    
                    <!-- Tomcat 独立更新 -->
                    <button id="btnTomcat" class="btn-orange" onclick="startScript('update', 'tomcat')" disabled>
                        <i class="fas fa-server"></i> 更新 Tomcat<br><span style="font-size:10px; opacity:0.8">(mdm.sh tomcat)</span>
                    </button>
                </div>

                <div id="deploy-term" style="height:400px;background:#000;border-radius:4px;"></div>
            </div>
        </div>
    </div>

    <div id="panel-files" class="panel"><div class="container-box" style="max-width: 1000px;"><div class="card" style="height:100%;padding:0"><div style="padding:15px;background:#f8f9fa;border-bottom:1px solid #eee"><div class="fm-toolbar"><button onclick="fmUpDir()">上级</button><button onclick="fmRefresh()">刷新</button><span id="fmPath" style="margin:0 10px;font-weight:bold">/root</span><input type="file" id="fmUploadInput" style="display:none" onchange="fmDoUpload()"><button onclick="document.getElementById('fmUploadInput').click()">上传</button></div><div id="fmStatus" style="font-size:12px;color:#666;height:15px"></div></div><div class="fm-list" style="overflow:auto;height:100%"><table style="width:100%"><tbody id="fmBody"></tbody></table></div></div></div></div>
    <div id="panel-terminal" class="panel"><div id="sys-term" class="full-term" style="height:100vh"></div></div>
    <div id="panel-logs" class="panel" style="padding:20px;height:100%"><div class="log-layout"><div class="log-sidebar"><div class="log-sidebar-header">日志列表</div><ul class="log-list"><li class="log-item" onclick="viewLog('tomcat', this)"><span>Tomcat</span> <button class="btn-dl-log" onclick="dlLog('tomcat', event)"><i class="fas fa-download"></i></button></li><li class="log-item" onclick="viewLog('nginx_access', this)"><span>Nginx Access</span> <button class="btn-dl-log" onclick="dlLog('nginx_access', event)"><i class="fas fa-download"></i></button></li><li class="log-item" onclick="viewLog('nginx_error', this)"><span>Nginx Error</span> <button class="btn-dl-log" onclick="dlLog('nginx_error', event)"><i class="fas fa-download"></i></button></li><li class="log-item" onclick="viewLog('app_server', this)"><span>App Server</span> <button class="btn-dl-log" onclick="dlLog('app_server', event)"><i class="fas fa-download"></i></button></li><li class="log-item" onclick="viewLog('emm_backend', this)"><span>EMM Backend</span> <button class="btn-dl-log" onclick="dlLog('emm_backend', event)"><i class="fas fa-download"></i></button></li><li class="log-item" onclick="viewLog('license', this)"><span>License</span> <button class="btn-dl-log" onclick="dlLog('license', event)"><i class="fas fa-download"></i></button></li><li class="log-item" onclick="viewLog('platform', this)"><span>Platform</span> <button class="btn-dl-log" onclick="dlLog('platform', event)"><i class="fas fa-download"></i></button></li></ul></div><div class="log-viewer-container"><div class="log-viewer-header"><span id="logTitle">请选择...</span><div><label><input type="checkbox" id="autoScroll" checked> 自动滚动</label> <button class="btn-sm" onclick="clearLog()">清空</button></div></div><div id="logContent" class="log-content"></div></div></div></div>
    
    <div id="panel-baseservices" class="panel">
        <div class="bs-header">
            <button class="sub-tab-btn active" onclick="switchSubTab(event, 'bs-redis')">Redis</button>
            <button class="sub-tab-btn" onclick="switchSubTab(event, 'bs-mysql')">MySQL</button>
            <button class="sub-tab-btn" onclick="switchSubTab(event, 'bs-rabbitmq')">RabbitMQ</button>
            <button class="sub-tab-btn" onclick="switchSubTab(event, 'bs-minio')">MinIO</button>
        </div>
        
        <div id="bs-redis" class="sub-panel active" style="padding: 20px; overflow-y: auto;">
            <div class="container-box" style="padding:0">
              <div class="card">
                 <h3>Redis 性能指标</h3>
                 <div id="redis-info-grid" class="grid-4">加载中...</div>
              </div>
              <div class="card">
                 <h3>键值管理</h3>
                 <div id="redis-keys-table-container">加载中...</div>
              </div>
            </div>
        </div>
        <div id="bs-mysql" class="sub-panel" style="padding: 20px; overflow-y: auto;">
            <div class="container-box" style="padding:0">
              <div class="card">
                 <div style="display:flex; align-items:center; gap:15px; margin-bottom:15px;">
                    <h3>MySQL 监控</h3>
                    <select id="db-selector" onchange="mysql.switchDB(this.value)"><option value="mdm">mdm</option><option value="multitenant">multitenant</option></select>
                    <button class="sub-tab-btn active" onclick="switchSubTab(event, 'mysql-monitor', false, 'mysql-tab-group')">监控</button>
                    <button class="sub-tab-btn" onclick="switchSubTab(event, 'mysql-sql', false, 'mysql-tab-group')">SQL执行</button>
                 </div>
                 <div id="mysql-monitor" class="mysql-tab-group active">
                    <div class="grid-4" style="margin-bottom: 15px;">
                       <div class="card"><h3>Threads</h3><div id="mysql-threads" style="font-size:1.5em;font-weight:bold;">0</div></div>
                       <div class="card"><h3>QPS</h3><div id="mysql-qps" style="font-size:1.5em;font-weight:bold;">0</div></div>
                       <div class="card"><h3>Max Connections</h3><div id="mysql-connections" style="font-size:1.5em;font-weight:bold;">0</div></div>
                       <div class="card"><h3>Uptime</h3><div id="mysql-uptime" style="font-size:1.5em;font-weight:bold;">0</div></div>
                    </div>
                    <div class="grid-2">
                       <div class="card"><h3>性能</h3><canvas id="mysql-metricChart"></canvas></div>
                       <div class="card"><h3>主从复制</h3><div id="mysql-replStatus"></div><canvas id="mysql-replChart"></canvas></div>
                       <div class="card"><h3>表空间占用 (Top 10)</h3><canvas id="mysql-tableSizeChart"></canvas></div>
                       <div class="card"><h3>频繁操作表 (Top 10)</h3><canvas id="mysql-tableOpsChart"></canvas></div>
                    </div>
                    <div class="card">
                       <h3>当前进程</h3>
                       <input id="mysql-slowFilter" placeholder="过滤SQL..." oninput="mysql.loadProcesslist()">
                       <div style="max-height: 400px; overflow-y: auto;"><table id="mysql-slowQueryTable"><thead><tr><th>Id</th><th>User</th><th>Host</th><th>DB</th><th>Command</th><th>Time(s)</th><th>State</th><th>Info</th></tr></thead><tbody></tbody></table></div>
                    </div>
                 </div>
                 <div id="mysql-sql" class="mysql-tab-group" style="display:none;">
                    <h3>执行SQL</h3>
                    <textarea id="mysql-sqlInput" rows="5" style="width:100%; font-family:monospace;"></textarea>
                    <button onclick="mysql.execSQL()" class="btn-green" style="margin-top:10px;">执行</button>
                    <div id="mysql-sqlResult" class="sql-table-container"></div>
                 </div>
              </div>
            </div>
        </div>
        <div id="bs-rabbitmq" class="sub-panel" style="padding: 0;">
            <iframe id="frame-rabbitmq" data-src="api/baseservices/rabbitmq/" class="iframe-container"></iframe>
        </div>
        <div id="bs-minio" class="sub-panel" style="padding: 0;">
            <iframe id="frame-minio" data-src="api/baseservices/minio/" class="iframe-container"></iframe>
        </div>
    </div>

    <div id="panel-about" class="panel">
        <div class="container-box" style="max-width: 800px;">
            <div class="card">
                <h3>关于 UEM Deployment Tools</h3>
                <table class="about-table">
                    <tbody>
                        <tr><td style="width: 100px;"><strong>作者</strong></td><td>王凯</td></tr>
                        <tr><td><strong>版本</strong></td><td>5.6 (Component Update)</td></tr>
                        <tr><td><strong>更新日期</strong></td><td>2024-07-26</td></tr>
                        <tr><td style="vertical-align: top; padding-top: 12px;"><strong>主要功能</strong></td><td><ul style="margin:0; padding-left: 20px; line-height: 1.8;"><li>系统基础环境、安全配置、服务状态一键体检</li><li>实时系统资源（内存/负载/网络）监控图表</li><li>通过上传或本地路径挂载 ISO 镜像，自动配置 YUM 源</li><li>在线安装 RPM 依赖包</li><li><strong>新功能：WebUI 和 Tomcat 独立更新支持</strong></li><li>指定服务器目录进行部署/更新（免重复上传）</li><li>全功能网页 Shell 终端 (Fix PTY)</li><li>实时查看多种 UEM 服务日志</li><li>基础服务(Redis/MySQL/RabbitMQ/MinIO)监控与管理</li></ul></td></tr>
                    </tbody>
                </table>
            </div>
        </div>
    </div>

    <div id="panel-help" class="panel">
        <div class="container-box">
            <div id="help-content" class="card" style="padding: 20px 30px;"></div>
        </div>
    </div>
</div>

<div id="modal-backdrop" class="modal-backdrop"></div>
<div id="modal" class="modal">
    <div class="modal-header"><h2 id="modal-title" class="modal-title"></h2><button id="modal-close-btn" class="modal-close">&times;</button></div>
    <div id="modal-body" class="modal-body"></div>
    <div class="modal-footer"><button type="button" id="modal-cancel-btn" class="btn-sm">关闭</button></div>
</div>

<script src="https://cdn.jsdelivr.net/npm/xterm@5.3.0/lib/xterm.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/xterm-addon-fit@0.8.0/lib/xterm-addon-fit.min.js"></script>
<script>
    let deployTerm, sysTerm, deploySocket, sysSocket, deployFit, sysFit, logSocket, currentPath = "/root";
    let sysChart, netChart; let checkInterval;

    // Fixed: Replaced double backslashes \\n with single backslash \n to ensure correct newline parsing in Markdown
    const readmeContent = '# UEM Deployment Tools\n\n> 一个用于UEM（统一端点管理）部署、监控和维护的辅助工具集。\n\n该工具包含一个在 Windows 上运行的客户端 (\x60uemtools.exe\x60) 和一个在目标 Linux 服务器上运行的代理 (\x60cncyagent\x60)。用户通过客户端与远程服务器上的代理进行交互，实现各种自动化操作和监控。\n\n## 🚀 产品功能 (Product Features)\n\n工具的前端界面提供了丰富的功能，主要分为以下几个模块：\n\n#### 1. 🔍 操作系统\n- **实时资源监控**: 以图表形式实时展示 CPU、内存使用率、系统平均负载和网络流量 (Rx/Tx)。\n- **基础环境检测**: 一键检查 CPU 核心数、内存大小、系统架构、操作系统版本和 \x60ulimit\x60 限制，并给出是否满足建议规格的判断。\n- **磁盘空间概览**: 以进度条形式清晰展示所有挂载点的磁盘使用情况。\n- **安全与网络**:\n    - 检查并提供一键修复 SELinux 和防火墙状态的功能。\n    - 检查并提供一键开启 SSH 隧道转发 (\x60AllowTcpForwarding\x60) 的功能。\n    - **网络端口**: 实时显示 \x60netstat -nltp\x60 的结果，方便查看端口监听状态。\n    - **TCP 连接数**: 实时统计当前系统的总 TCP 连接数量。\n- **UEM 服务监控**: 自动检测 UEM 是否安装，并实时展示所有相关服务的运行状态，提供单独重启服务的功能。\n- **MinIO 检测**: 检查 MinIO 存储桶是否存在以及其访问策略是否为公开，并提供一键修复为公开读策略的功能。\n\n#### 2. 🔧 环境依赖\n- **ISO 挂载**: 支持通过上传 ISO 镜像文件或指定服务器上的本地路径，自动将其挂载并配置为本地 YUM 源。\n- **RPM 安装**: 支持在线上传 RPM 包并直接在服务器上进行安装。\n\n#### 3. 📦 部署/更新\n- **灵活的部署路径**: 支持指定任意服务器目录作为工作目录。\n- **脚本自动检测**: 自动检测指定目录下是否存在 \x60install.sh\x60 (用于首次部署) 和 \x60mdm.sh\x60 (用于更新)，并根据检测结果动态启用相关操作按钮。\n- **一键式操作**:\n    - **首次部署**: 执行 \x60install.sh\x60。\n    - **UEM 更新**: 执行 \x60mdm.sh uem\x60。\n    - **WebUI 更新**: 执行 \x60mdm.sh webui\x60。\n    - **Tomcat 更新**: 执行 \x60mdm.sh tomcat\x60。\n- **实时输出**: 所有部署和更新操作的输出都会实时显示在网页终端中。\n\n#### 4. 📂 文件管理\n- 提供一个简单的网页版文件浏览器。\n- 支持在服务器上浏览目录、返回上级。\n- 支持从本地上传文件到服务器的当前目录。\n- 支持下载服务器上的文件。\n\n#### 5. 💻 终端\n- 提供一个功能完整的、交互式的网页 Shell 终端，可以直接操作服务器。\n\n#### 6. 📜 日志查看\n- **多日志源**: 预设了 Tomcat, Nginx, AppServer 等多个常用服务的日志。\n- **实时流式传输**: 点击即可实时查看日志 (\x60tail -f\x60)。\n- **便捷操作**: 支持自动滚动、清空显示和一键下载日志文件。\n\n#### 7. ⚙️ 基础服务\n- **零配置监控**: 工具能够**自动读取** UEM 的核心配置文件 \x60global.properties\x60，从中获取 Redis、MySQL、RabbitMQ 等服务的连接信息，无需用户手动输入。\n- **Redis**:\n    - 实时查看 Redis 的各项性能指标。\n    - 浏览、查看、修改和删除 Key-Value。\n- **MySQL**:\n    - 实时监控多实例 (mdm, multitenant) 的 QPS、线程数、连接数、主从延迟等状态。\n    - 以图表形式展示表空间和表操作的 Top 10。\n    - 查看实时进程列表 (\x60SHOW FULL PROCESSLIST\x60)。\n    - 提供 SQL 执行器，可直接执行查询或修改语句并查看结果。\n- **RabbitMQ & MinIO**:\n    - 通过反向代理将它们的管理后台无缝内嵌到工具中，无需再次登录或暴露端口。\n\n## 🛠️ 技术实现 (Technical Implementation)\n\n#### 1. 项目架构\n本项目采用 **C/S 架构**，并通过 **Wails** 技术将客户端和服务端能力打包成一个独立的 Windows 桌面应用。\n\n- **Agent (后端代理)**: 一个纯 Go 语言编写的轻量级 Web 服务器，运行在目标 Linux 服务器上。它不依赖任何外部运行时，具有出色的跨平台和性能表现。\n- **Client (前端客户端)**: 一个基于 **Wails v2** 的 Windows 桌面应用。Wails 使用 Go 作为后端，通过 WebView2 渲染前端 UI，实现了接近本机的性能和体验。\n\n---\n\n#### 2. Agent 核心实现方法\n\nAgent 是所有远程操作的执行者。\n\n- **Web 服务**:\n    - 使用 **Gin** (\x60github.com/gin-gonic/gin\x60) 框架构建，它以高性能和简洁的 API 设计著称。\n    - Agent 启动后，会监听一个端口（默认为 \x609898\x60），提供两类服务：\n        1.  **RESTful API**: 用于处理无状态、一次性的请求，如获取系统信息 (\x60/api/check\x60)、列出文件 (\x60/api/fs/list\x60) 等。\n        2.  **WebSocket 服务**: 用于需要持久连接和实时双向通信的场景。\n\n- **系统信息与服务发现**:\n    - **命令执行**: 大部分系统信息通过 \x60os/exec\x60 包执行标准的 Linux 命令并解析其 \x60stdout\x60 来获得。例如：\n        - \x60netstat -nltp\x60: 获取监听中的 TCP 端口信息。Go 代码会逐行解析输出，提取协议、地址、PID 等关键字段，并将其构造成 JSON 数组返回给前端。\n        - \x60df -h\x60: 获取磁盘分区信息。\n        - \x60ulimit -n\x60: 获取文件句柄数限制。\n    - **文件读取**: 部分核心指标（如内存、CPU）直接读取 \x60/proc\x60 文件系统下的文件（如 \x60/proc/meminfo\x60, \x60/proc/loadavg\x60）来获取原始数据，这样做比执行命令更高效。\n    - **自动服务发现 (通过 \x60global.properties\x60)**:\n        - **定位文件**: Agent 启动时，会自动尝试读取 UEM 的核心配置文件 \x60/opt/emm/current/config/global.properties\x60。\n        - **解析配置**: 使用 \x60github.com/magiconair/properties\x60 库解析该 \x60.properties\x60 文件。\n        - **提取关键信息**: 从文件中读取以下关键配置项：\n            - \x60system.redis.host\x60, \x60system.redis.port\x60, \x60system.redis.password\x60\n            - \x60jdbc.url\x60, \x60jdbc.username\x60, \x60jdbc.password\x60 (用于主数据库 \x60mdm\x60)\n            - \x60jdbc.multitenant.url\x60, \x60jdbc.multitenant.username\x60, \x60jdbc.multitenant.password\x60 (用于多租户数据库)\n            - \x60spring.rabbitmq.addresses\x60, \x60rabbitmq.admin.port\x60\n            - \x60storage.minio.url\x60\n        - **建立连接**: 基于这些提取出的信息，Agent 初始化到各个服务的连接池（如 \x60database/sql\x60 和 \x60go-redis\x60），从而实现了对这些基础服务的“零配置”监控和管理。\n\n- **交互式终端与实时日志**:\n    - **伪终端 (PTY)**: 使用 \x60github.com/creack/pty\x60 库在服务器上创建一个伪终端，并将用户的 Shell（如 \x60/bin/bash\x60）附加到该终端上。\n    - **WebSocket 桥接**:\n        1.  前端（使用 **Xterm.js**）捕获用户的按键输入，通过 WebSocket 将数据发送到 Agent。\n        2.  Agent 接收到数据后，将其写入 PTY 的 master 端，模拟真实终端的输入。\n        3.  Shell 在 PTY 的 slave 端执行命令，其输出被 Agent 从 PTY 的 master 端读取。\n        4.  Agent 将读取到的输出通过 WebSocket 实时地回传给前端的 Xterm.js 进行显示。\n    - **实时日志** (\x60tail -f\x60) 的实现与此类似，只是将 \x60bash\x60 换成了 \x60tail -f /path/to/log\x60 命令。\n\n---\n\n#### 3. Client 与 Agent 的通信流程\n\n这是整个工具链的核心。\n\n1.  **启动与连接**:\n    - 用户在 Wails 应用中输入服务器的 IP、用户名和密码。\n    - Wails 的 Go 后端使用 \x60golang.org/x/crypto/ssh\x60 包建立一个到目标服务器的 SSH 连接。\n\n2.  **Agent 部署与启动**:\n    - 项目在编译时，已通过 Go 的交叉编译功能将 Agent 代码编译成 \x60amd64\x60 和 \x60arm64\x60 两种架构的 Linux 二进制文件，并将其**嵌入**到 Wails 客户端 \x60uemtools.exe\x60 中。\n    - SSH 连接成功后，客户端首先检测服务器的架构 (\x60uname -m\x60)。\n    - 根据架构，选择对应的 Agent 二进制文件，通过 SSH 的 SFTP 功能将其上传到服务器的一个临时目录（如 \x60/tmp/cncyagent_amd64\x60）。\n    - 客户端通过 SSH 执行命令，为 Agent 添加执行权限 (\x60chmod +x\x60)，然后启动它。\n\n3.  **建立通信隧道 (SSH 端口转发)**:\n    - Agent 在服务器上监听的是 \x60127.0.0.1:9898\x60，这是一个仅限服务器本地访问的地址，保证了安全性。\n    - 为了让 Wails 客户端（运行在用户的 Windows 电脑上）能够访问到这个端口，客户端在建立 SSH 连接的同时，会设置一个**本地端口转发** (Local Port Forwarding)。\n    - 这意味着，客户端会监听自己电脑上的一个端口（例如 \x60localhost:39898\x60），并将所有发送到该端口的流量通过 SSH 安全隧道转发到服务器的 \x60127.0.0.1:9898\x60。\n\n4.  **前端交互**:\n    - Wails 应用的前端 UI 是一个内嵌的网页。这个网页中的所有 API 请求（通过 \x60fetch\x60）和 WebSocket 连接，实际上访问的都是 \x60http://localhost:39898\x60。\n    - 这些请求被 SSH 客户端拦截，加密后通过隧道发送到服务器，最终到达 Agent。Agent 处理后返回的数据再沿着隧道传回前端。\n    - 对用户和前端代码来说，整个过程是透明的，就像在直接访问一个本地服务。\n\n---\n\n#### 4. 反向代理的实现\n\n为了将 RabbitMQ 和 MinIO 的 Web 管理后台无缝集成，Agent 内置了一个基于 \x60net/http/httputil.NewSingleHostReverseProxy\x60 的反向代理。\n\n- **解决跨域和端口问题**: 无需将 RabbitMQ/MinIO 的管理端口暴露到公网，前端只需与 Agent 的端口通信。\n- **路径重写**: 代理会拦截特定路径的请求（如 \x60/api/baseservices/rabbitmq/*\x60)，将请求转发到内部的管理端口（如 \x60127.0.0.1:15672\x60)，并将响应返回给前端。\n- **HTML 内容修复**: 由于这些管理后台的 HTML/JS/CSS 文件中的资源路径通常是绝对路径（如 \x60/css/main.css\x60)，直接在 iframe 中加载会导致 404。代理会在返回响应前，读取 HTML/JS 内容，通过字符串替换，将这些路径动态修改为带代理前缀的路径（如 \x60src="/css/main.css"\x60 -> \x60src="/api/baseservices/rabbitmq/css/main.css"\x60)，从而确保所有资源都能被正确加载。\n\n## 🏗️ 如何构建 (How to Build)\n\n项目提供了一个 \x60build.sh\x60 脚本来自动化整个构建流程。\n\n1.  确保您的开发环境中已安装 Go 和 Wails v2。\n2.  在项目根目录下，执行以下命令：\n    \x60\x60\x60sh\n    sh build.sh\n    \x60\x60\x60\n3.  脚本会完成以下工作：\n    - 交叉编译生成两个 Linux Agent: \x60cncyagent_amd64\x60 和 \x60cncyagent_arm64\x60。\n    - 使用 Wails 构建 Windows 客户端 \x60uemtools.exe\x60，并将上述 Agent 二进制文件嵌入其中。\n    - 将所有生成物移动到 \x60build/bin/\x60 目录下。\n\n构建成功后，所有产物都位于 \x60build/bin/\x60 目录中。\n\n## Nginx 配置示例 (Nginx Configuration Example)\n\n如果您希望通过 Nginx 反向代理来访问 Agent，可以使用以下配置：\n\n\x60\x60\x60nginx\nlocation /gogogo/ {\n    # 注意端口号改成你的 agent 端口，末尾的 / 用于去除 /gogogo 前缀\n    proxy_pass http://127.0.0.1:9898/;\n   \n    proxy_set_header Host $host;\n    proxy_set_header X-Real-IP $remote_addr;\n    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n    proxy_set_header X-Forwarded-Prefix /gogogo;\n    # 支持 WebSocket\n    proxy_http_version 1.1;\n    proxy_set_header Upgrade $http_upgrade;\n    proxy_set_header Connection "upgrade";\n}\n\x60\x60\x60\n';

    window.onload = function() {
         initCharts();
         runCheck();
         fmLoadPath("/root");
         startCheckPolling();
         document.getElementById('help-content').innerHTML = marked.parse(readmeContent);
    }
    function startCheckPolling() { if(checkInterval) clearInterval(checkInterval); checkInterval = setInterval(() => { if(document.getElementById('panel-check').classList.contains('active')) { runCheck(); } }, 3000); }
    function initCharts() {
        const ctx = document.getElementById('sysChart').getContext('2d');
        sysChart = new Chart(ctx, { type: 'line', data: { labels: [], datasets: [ { label: '内存使用率 (%)', data: [], borderColor: '#e74c3c', backgroundColor: 'rgba(231, 76, 60, 0.1)', fill: true, tension: 0.3 }, { label: '系统负载 (1min) - CPU活跃进程', data: [], borderColor: '#2980b9', backgroundColor: 'rgba(41, 128, 185, 0.1)', fill: true, tension: 0.3, yAxisID: 'y1' } ] }, options: { responsive: true, maintainAspectRatio: false, animation: false, interaction: { mode: 'index', intersect: false, }, scales: { y: { beginAtZero: true, max: 100, title: { display: true, text: 'Memory %' } }, y1: { type: 'linear', display: true, position: 'right', beginAtZero: true, title: { display: true, text: 'Load Avg' }, grid: { drawOnChartArea: false, }, }, x: { ticks: { display: false } } } } });
        const ctx2 = document.getElementById('netChart').getContext('2d');
        netChart = new Chart(ctx2, { type: 'line', data: { labels: [], datasets: [ { label: 'Rx (下载)', data: [], borderColor: '#27ae60', fill: false, tension: 0.3 }, { label: 'Tx (上传)', data: [], borderColor: '#f39c12', fill: false, tension: 0.3 } ] }, options: { responsive: true, maintainAspectRatio: false, animation: false, scales: { y: { beginAtZero: true, title: { display: true, text: 'KB/s' } }, x: { ticks: { display: false } } } } });
    }
    function switchTab(id) {
        document.querySelectorAll('.panel').forEach(p => p.classList.remove('active')); document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
        document.getElementById('panel-'+id).classList.add('active'); event.target.classList.add('active');
        if (id === 'terminal') { if (!sysTerm) initSysTerm(); setTimeout(()=>sysFit.fit(), 200); }
        if (id === 'deploy') { setTimeout(()=>deployFit && deployFit.fit(), 200); }
        if (id === 'baseservices') { redis.init(); mysql.init(); }
    }
    function switchSubTab(event, id, isLink, group) {
       if (isLink) { document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active')); const mainBtn = Array.from(document.querySelectorAll('.tab-btn')).find(b => b.textContent.includes('基础服务')); if(mainBtn) mainBtn.classList.add('active'); document.querySelectorAll('.panel').forEach(p => p.classList.remove('active')); document.getElementById('panel-baseservices').classList.add('active'); }
       if(group) { const p = event.target.closest('.card'); p.querySelectorAll('.'+group).forEach(x=>x.style.display='none'); p.querySelectorAll('.sub-tab-btn').forEach(b=>b.classList.remove('active')); document.getElementById(id).style.display='block'; event.target.classList.add('active'); return; }
        else { const parent = document.getElementById('panel-baseservices'); parent.querySelectorAll('.sub-panel').forEach(p => p.classList.remove('active')); parent.querySelectorAll('.sub-tab-btn').forEach(b => b.classList.remove('active')); document.getElementById(id).classList.add('active'); event.target.classList.add('active'); }
       if (id === 'bs-rabbitmq') { const frame = document.getElementById('frame-rabbitmq'); if (!frame.src) frame.src = frame.dataset.src; }
        else if (id === 'bs-minio') { const frame = document.getElementById('frame-minio'); if (!frame.src) { frame.src = frame.dataset.src; frame.onload = function() { let attempts = 0; const interval = setInterval(() => { attempts++; if(attempts > 40) clearInterval(interval); try { const doc = frame.contentWindow.document; const user = doc.getElementById('accessKey'); const pass = doc.getElementById('secretKey'); const btn = doc.querySelector('button[type="submit"]'); if(user && pass && btn) { const nativeInputValueSetter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value").set; nativeInputValueSetter.call(user, 'admin'); user.dispatchEvent(new Event('input', { bubbles: true })); nativeInputValueSetter.call(pass, 'Nqsky1130'); pass.dispatchEvent(new Event('input', { bubbles: true })); setTimeout(() => { btn.click(); }, 300); clearInterval(interval); } } catch(e) {} }, 500); }; } }
    }
    function getWsUrl(ep) { let path = location.pathname; if (!path.endsWith('/')) path += '/'; return (location.protocol==='https:'?'wss://':'ws://') + location.host + path + ep; }
    function viewLog(key, el) {
        document.querySelectorAll('.log-item').forEach(l=>l.classList.remove('active')); el.classList.add('active'); document.getElementById('logTitle').innerText = "Log: " + key;
        const box = document.getElementById('logContent'); box.innerText = "Connecting...\n";
        if(logSocket) logSocket.close();
        logSocket = new WebSocket(getWsUrl("ws/log?key="+key));
        logSocket.onmessage = e => { box.innerText += e.data; if(box.innerText.length>50000) box.innerText=box.innerText.substring(box.innerText.length-50000); if(document.getElementById('autoScroll').checked) box.scrollTop=box.scrollHeight; };
        logSocket.onclose = () => { box.innerText += "\n>>> Disconnected"; };
    }
    function dlLog(key, e) { e.stopPropagation(); window.location.href = 'api/log/download?key=' + key; }
    function clearLog(){ document.getElementById('logContent').innerText=""; }
    
    // ==========================================
    // 核心逻辑：目录检测与按钮控制
    // ==========================================
    async function checkManualPath() {
        const path = document.getElementById('manualPathInput').value.trim();
        const msgBox = document.getElementById('pathCheckMsg');
        
        const btnInstall = document.getElementById('btnInstall');
        const btnUEM = document.getElementById('btnUEM');
        const btnWebUI = document.getElementById('btnWebUI');
        const btnTomcat = document.getElementById('btnTomcat');

        // 先全部禁用
        [btnInstall, btnUEM, btnWebUI, btnTomcat].forEach(b => b.disabled = true);

        if (!path) {
            msgBox.innerHTML = '<span class="fail">请输入路径</span>';
            return;
        }
        msgBox.innerHTML = '正在检测...';

        try {
            const res = await fetch('api/check_dir?path=' + encodeURIComponent(path));
            const data = await res.json();

            if (data.exists) {
                let info = '<span class="pass">目录存在。</span> ';
                let foundScript = false;

                if (data.has_install) {
                    btnInstall.disabled = false;
                    info += '✅ install.sh ';
                    foundScript = true;
                }

                if (data.has_mdm) {
                    btnUEM.disabled = false;
                    btnWebUI.disabled = false;
                    btnTomcat.disabled = false;
                    info += '✅ mdm.sh (支持更新) ';
                    foundScript = true;
                }

                if (!foundScript) { 
                    info += '<span class="warn">未找到 install.sh 或 mdm.sh</span><br><span class="fail" style="font-size:11px;">' + (data.debug_msg||"") + '</span>';
                }
                msgBox.innerHTML = info;
            } else {
                msgBox.innerHTML = '<span class="fail">目录不存在 (' + (data.debug_msg || "") + ')</span>';
            }
        } catch (e) {
            console.error(e);
            msgBox.innerHTML = '<span class="fail">检测请求失败</span>';
        }
    }

    // [MODIFIED] Replaced old uploadFile with new version supporting progress bar
    function uploadFileWithProgress() {
        const fileInput = document.getElementById('fileInput');
        const pathInput = document.getElementById('uploadPathInput');
        const uploadButton = document.getElementById('uploadButton');
        const statusDiv = document.getElementById('uploadStatus');
        const progressContainer = document.getElementById('uploadProgressContainer');
        const progressBar = document.getElementById('uploadProgressBar');

        if (fileInput.files.length === 0) {
            alert('请先选择一个文件');
            return;
        }

        const file = fileInput.files[0];
        const path = pathInput.value.trim();
        const formData = new FormData();
        formData.append('file', file);
        formData.append('path', path);

        uploadButton.disabled = true;
        statusDiv.innerHTML = '准备上传...';
        progressContainer.style.display = 'block';
        progressBar.style.width = '0%';
        progressBar.textContent = '';

        const xhr = new XMLHttpRequest();
        xhr.open('POST', "upload", true);

        xhr.upload.onprogress = function(event) {
            if (event.lengthComputable) {
                const percentComplete = Math.round((event.loaded / event.total) * 100);
                progressBar.style.width = percentComplete + '%';
                progressBar.textContent = percentComplete + '%';
                statusDiv.innerHTML = '正在上传... ' + percentComplete + '%';
            }
        };

        xhr.onload = function() {
            uploadButton.disabled = false;
            if (xhr.status === 200) {
                progressBar.style.backgroundColor = '#27ae60';
                statusDiv.innerHTML = '<span class="pass">✅ 上传并处理成功!</span>';
                try {
                    const response = JSON.parse(xhr.responseText);
                    alert('成功: ' + response.message);
                } catch (e) {
                    alert('上传成功，但服务器响应异常。');
                }
                // 自动检测工作目录，以启用脚本按钮
                checkManualPath();
            } else {
                progressBar.style.backgroundColor = '#c0392b';
                statusDiv.innerHTML = '<span class="fail">❌ 上传失败!</span>';
                try {
                    const response = JSON.parse(xhr.responseText);
                    alert('失败: ' + (response.error || '未知错误') + '\\n' + (response.details || ''));
                } catch (e) {
                    alert('上传失败: ' + xhr.statusText);
                }
            }
        };

        xhr.onerror = function() {
            uploadButton.disabled = false;
            progressBar.style.backgroundColor = '#c0392b';
            statusDiv.innerHTML = '<span class="fail">❌ 网络错误!</span>';
            alert('上传失败: 无法连接到服务器。');
        };
        
        xhr.send(formData);
    }
    
    // Keep the old uploadFile function for compatibility if needed, or remove it.
    // For this change, we are replacing its call, so it's not strictly necessary.
    async function uploadFile() {
        alert("此功能已被新版上传取代，请使用带进度条的上传。");
    }


    // ==========================================
    // 核心逻辑：执行脚本 (带参数)
    // ==========================================
    function startScript(type, arg) {
         const path = document.getElementById('manualPathInput').value.trim();
        
         if(deployTerm) deployTerm.dispose();
         if(deploySocket) deploySocket.close();
         
         deployTerm = new Terminal({cursorBlink:true, fontSize:13, theme:{background:'#000'}});
         deployFit = new FitAddon.FitAddon();
         deployTerm.loadAddon(deployFit);
         deployTerm.open(document.getElementById('deploy-term'));
         deployFit.fit();
         
         // 构建 WebSocket URL
        let wsUrl = "ws/deploy?type=" + type + "&path=" + encodeURIComponent(path);
        if (arg) {
            wsUrl += "&arg=" + arg;
        }
        
         deploySocket = new WebSocket(getWsUrl(wsUrl));
         setupSocket(deploySocket, deployTerm, deployFit);
         
         // 执行中禁用按钮
        const btns = document.querySelectorAll('#panel-deploy button');
        btns.forEach(b => b.disabled = true);
    }

    function initSysTerm() { sysTerm=new Terminal({cursorBlink:true,fontSize:14,fontFamily:'Consolas, monospace'}); sysFit=new FitAddon.FitAddon(); sysTerm.loadAddon(sysFit); sysTerm.open(document.getElementById('sys-term')); sysFit.fit(); sysSocket=new WebSocket(getWsUrl("ws/terminal")); setupSocket(sysSocket, sysTerm, sysFit); }
    function setupSocket(s, t, f) { s.onopen=()=>{s.send(JSON.stringify({type:"resize",cols:t.cols,rows:t.rows}));f.fit()}; s.onmessage=e=>t.write(e.data); t.onData(d=>{if(s.readyState===1)s.send(JSON.stringify({type:"input",data:d}))}); window.addEventListener('resize',()=>{f.fit();if(s.readyState===1)s.send(JSON.stringify({type:"resize",cols:t.cols,rows:t.rows}))}); }
    function escapeHtml(unsafe) { return unsafe ? unsafe.toString().replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;") : ''; }

    // --- Restored Missing Functions ---
    async function runCheck() {
        try {
            const resp = await fetch('api/check'); const data = await resp.json();
            if(sysChart && data.sys_info.mem_usage !== undefined) {
                const now = new Date().toLocaleTimeString();
                if(sysChart.data.labels.length > 20) { sysChart.data.labels.shift(); sysChart.data.datasets.forEach(d => d.data.shift()); netChart.data.labels.shift(); netChart.data.datasets.forEach(d => d.data.shift()); }
                sysChart.data.labels.push(now); sysChart.data.datasets[0].data.push(data.sys_info.mem_usage); sysChart.data.datasets[1].data.push(data.sys_info.load_avg); sysChart.update();
                netChart.data.labels.push(now); netChart.data.datasets[0].data.push(data.sys_info.net_rx || 0); netChart.data.datasets[1].data.push(data.sys_info.net_tx || 0); netChart.update();
            }
            let baseHtml = '';
            baseHtml += row('CPU', data.sys_info.cpu_cores + ' 核', data.sys_info.cpu_pass); baseHtml += row('内存', data.sys_info.mem_total, data.sys_info.mem_pass); baseHtml += row('架构', data.sys_info.arch, true); baseHtml += row('操作系统', data.sys_info.os_name, data.sys_info.os_pass);
            baseHtml += '<tr><td>性能(ulimit)</td><td>'+data.sys_info.ulimit+'</td><td>'+(data.sys_info.ulimit_pass?'<span class="pass">OK</span>':'<span class="warn">Opt</span>')+'</td></tr>';
            document.getElementById('baseTable').innerHTML = baseHtml;
            let secHtml = '';
            secHtml += '<tr><td>SELinux</td><td>'+data.sec_info.selinux+'</td><td>'+(data.sec_info.selinux==="Disabled"||data.sec_info.selinux==="Permissive"?'<span class="pass">OK</span>':'<button class="btn-sm btn-fix" onclick="fixSelinux()">⛔ 关闭</button>')+'</td></tr>';
            
            let fwStatus = data.sec_info.firewall;
            let fwDisplay = (fwStatus === 'Stopped' || fwStatus === 'Off') ? '<span class="pass">OK</span>' : '<button class="btn-sm btn-fix" onclick="fixFirewall()">⛔ 关闭</button>';
            secHtml += '<tr><td>防火墙</td><td>'+fwStatus+'</td><td>'+fwDisplay+'</td></tr>';

            let sshBtn = data.sec_info.ssh_tunnel_ok ? '<span class="pass">开启</span>' : '<span class="fail">关闭</span> <button class="btn-sm btn-fix" onclick="fixSsh()">🔧 修复</button>';
            secHtml += '<tr><td>SSH隧道</td><td>TCP转发</td><td>'+sshBtn+'</td></tr>';
            document.getElementById('secTable').innerHTML = secHtml;
            let diskHtml = '<div style="display:flex; flex-direction:column; gap:12px;">';
            data.sys_info.disk_list.forEach(d => { let color = d.usage>=90?'bg-red':(d.usage>=75?'bg-orange':'bg-green'); diskHtml += '<div><div style="font-weight:bold;margin-bottom:4px;font-size:13px;">'+d.mount+' <span style="color:#666">('+d.usage+'%)</span></div><div class="progress-bg"><div class="progress-bar '+color+'" style="width:'+d.usage+'%"></div></div><div class="disk-text"><span>'+d.used+'</span><span>'+d.total+'</span></div></div>'; });
            document.getElementById('diskList').innerHTML = diskHtml + '</div>';
            const uemBox = document.getElementById('uemStatusBox');
            if (!data.uem_info.installed) { uemBox.innerHTML = '<div style="color:#7f8c8d;text-align:center;padding:20px;">未检测到 UEM</div>'; }
             else { let h = '<table style="width:100%"><thead><tr><th>服务</th><th>状态</th><th>操作</th></tr></thead><tbody>'; data.uem_info.services.forEach(s => { let st = s.status==='run'?'<span class="pass">running</span>':'<span class="fail">Stop</span>'; h += '<tr><td>'+s.name+'</td><td>'+st+'</td><td><button class="btn-sm btn-restart" onclick="restartService(\''+s.name+'\')">重启</button></td></tr>'; }); uemBox.innerHTML = h + '</tbody></table>'; }
            let mHtml = !data.minio_info.bucket_exists ? '<tr><td>Err</td><td colspan="2">桶不存在/未连接</td></tr>' : '<tr><td>nqsky</td><td>'+data.minio_info.policy+'</td><td>'+(data.minio_info.policy==='public'?'<span class="pass">OK</span>':'<button class="btn-sm btn-fix" onclick="fixMinio()">Public</button>')+'</td></tr>';
            document.getElementById('minioTable').innerHTML = mHtml;

            // Display network info
            document.getElementById('tcpConnCountBox').textContent = data.sys_info.tcp_conn_count || 'N/A';
            const netstatBody = document.getElementById('netstatTableBody');
            netstatBody.innerHTML = ''; // Clear previous data
            if (data.sys_info && data.sys_info.netstat && data.sys_info.netstat.length > 0) {
                data.sys_info.netstat.forEach(item => {
                    const row = document.createElement('tr');
                    row.innerHTML = '<td>' + escapeHtml(item.proto) + '</td>'
                                  + '<td>' + escapeHtml(item.address) + '</td>'
                                  + '<td>' + escapeHtml(item.pid) + '</td>';
                    netstatBody.appendChild(row);
                });
            } else {
                netstatBody.innerHTML = '<tr><td colspan="3" style="text-align:center; color:#999;">没有监视中的端口或获取失败</td></tr>';
            }

        } catch(e) {
            console.error("Error in runCheck:", e);
        }
    }
    
    function row(name, val, pass) { return '<tr><td>'+name+'</td><td>'+val+'</td><td>'+(pass?'<span class="pass">OK</span>':'<span class="fail">Fail</span>')+'</td></tr>'; }
    async function fixSelinux() { if(confirm("关闭 SELinux (需重启)？")) fetch('api/sec/selinux',{method:'POST'}).then(r=>r.text()).then(t=>{ alert(t); runCheck(); }); }
    async function fixFirewall() { if(confirm("关闭防火墙？")) fetch('api/sec/firewall',{method:'POST'}).then(r=>r.text()).then(alert).then(runCheck); }
    async function restartService(n) { if(confirm('重启 '+n+' ?')) fetch('api/service/restart?name='+n,{method:'POST'}).then(r=>r.text()).then(alert).then(runCheck); }
    async function fixMinio() { if(confirm("Public?")) fetch('api/minio/fix',{method:'POST'}).then(r=>r.text()).then(alert).then(runCheck); }
    async function fixSsh() { if(confirm("Fix SSH?")) fetch('api/fix_ssh',{method:'POST'}).then(r=>r.text()).then(alert); }
    
    async function fmLoadPath(p) { currentPath=p; document.getElementById('fmPath').innerText=p; const r=await fetch('api/fs/list?path='+encodeURIComponent(p)); const fs=await r.json(); let h=''; fs.sort((a,b)=>(a.is_dir===b.is_dir)?0:a.is_dir?-1:1); fs.forEach(f=>{ let n=f.is_dir?'<a class="link-dir" href="javascript:fmLoadPath(\''+f.path+'\')">'+f.name+'</a>':f.name; let act=f.is_dir?'':'<button class="btn-sm" onclick="fmDownload(\''+f.path+'\')">下载</button>'; h+='<tr><td>'+(f.is_dir?'📁':'📄')+' '+n+'</td><td>'+f.size+'</td><td>'+f.mod_time+'</td><td>'+act+'</td></tr>'; }); document.getElementById('fmBody').innerHTML=h; }
    function fmUpDir() { let p=currentPath.split('/'); p.pop(); let n=p.join('/'); if(!n)n='/'; fmLoadPath(n); }
    function fmDownload(p) { window.location.href = 'api/fs/download?path=' + encodeURIComponent(p); }
    function fmRefresh() { fmLoadPath(currentPath); }
    async function fmDoUpload() { const inp=document.getElementById('fmUploadInput'); const fd=new FormData(); fd.append("file", inp.files[0]); fd.append("path", currentPath); const st=document.getElementById('fmStatus'); st.innerText="Uploading..."; await fetch('api/upload_any', {method:'POST', body:fd}); st.innerText="Done"; fmLoadPath(currentPath); }
    
    async function mountIso() { const inp=document.getElementById('isoInput'); if(!inp.files.length)return; event.target.disabled=true; const fd=new FormData(); fd.append("file",inp.files[0]); const r=await fetch('api/iso_mount',{method:'POST',body:fd}); const rd=r.body.getReader(); const d=new TextDecoder(); const box=document.getElementById('yum-log'); while(true){const{done,value}=await rd.read();if(done)break;box.innerText+=d.decode(value);box.scrollTop=box.scrollHeight;} event.target.disabled=false; }
    async function mountLocalIso() { const p = document.getElementById('isoPathInput').value; if(!p) return alert("请输入路径"); event.target.disabled=true; const fd=new FormData(); fd.append("path", p); const r=await fetch('api/iso_mount_local',{method:'POST',body:fd}); const rd=r.body.getReader(); const d=new TextDecoder(); const box=document.getElementById('yum-log'); box.innerText = ">>> 正在使用本地文件挂载...\n"; while(true){const{done,value}=await rd.read();if(done)break;box.innerText+=d.decode(value);box.scrollTop=box.scrollHeight;} event.target.disabled=false; }
    async function installRpm() { const i=document.getElementById('rpmInput'); if(!i.files.length)return; event.target.disabled=true; const fd=new FormData(); fd.append("file",i.files[0]); const r=await fetch('api/rpm_install',{method:'POST',body:fd}); const rd=r.body.getReader(); const d=new TextDecoder(); const box=document.getElementById('rpm-log'); while(true){const{done,value}=await rd.read();if(done)break;box.innerText+=d.decode(value);box.scrollTop=box.scrollHeight;} event.target.disabled=false; }

    const redis = {
       allKeys: [], currentFilter: 'all', initialized: false,
       init: function() { if(this.initialized) return; this.fetchInfo(); this.fetchAllKeys(); this.initialized = true; },
       fetchInfo: async function() { try { const res = await fetch('api/baseservices/redis/info'); if (!res.ok) throw new Error('Failed to fetch info'); const info = await res.json(); const metrics = {'redis_version': 'Version', 'uptime_in_days': 'Uptime (Days)', 'connected_clients': 'Clients', 'used_memory_human': 'Memory', 'total_commands_processed': 'Commands', 'instantaneous_ops_per_sec': 'Ops/Sec'}; const grid = document.getElementById('redis-info-grid'); grid.innerHTML = ''; for (const key in metrics) { if (info[key]) grid.innerHTML += '<div class="card"><h3>' + metrics[key] + '</h3><p style="font-size:1.5em;font-weight:bold;">' + info[key] + '</p></div>'; } } catch (e) { document.getElementById('redis-info-grid').innerHTML = '<p class="fail">Failed to load Redis stats.</p>'; } },
       fetchAllKeys: async function() { try { const res = await fetch('api/baseservices/redis/keys'); if (!res.ok) throw new Error('Failed to fetch keys'); this.allKeys = await res.json() || []; this.allKeys.sort((a, b) => a.key.localeCompare(b.key)); this.renderTable(); } catch (e) { document.getElementById('redis-keys-table-container').innerHTML = '<p class="fail">Failed to load keys.</p>'; } },
       renderTable: function() { let html = '<table><thead><tr><th style="width:60%">Key</th><th style="width:15%">Type</th><th style="width:25%">Actions</th></tr></thead><tbody>'; this.allKeys.forEach(item => { html += '<tr><td class="redis-key-cell" title="' + escapeHtml(item.key) + '">' + escapeHtml(item.key) + '</td><td>' + escapeHtml(item.type) + '</td><td><button class="btn-sm" onclick="redis.viewEditKey(\'' + item.key + '\', \'' + item.type + '\')">View/Edit</button> <button class="btn-sm btn-red" onclick="redis.deleteKey(\'' + item.key + '\')">Delete</button></td></tr>'; }); html += '</tbody></table>'; document.getElementById('redis-keys-table-container').innerHTML = html; },
       deleteKey: async function(key) { if (!confirm('确认删除: ' + key + '?')) return; await fetch('api/baseservices/redis/key?key=' + encodeURIComponent(key), { method: 'DELETE' }); this.fetchAllKeys(); },
       viewEditKey: async function(key, type) { document.getElementById('modal-title').textContent = 'Editing ' + type + ': ' + key; document.getElementById('modal-body').innerHTML = '<p>Loading...</p>'; document.getElementById('modal-backdrop').style.display = 'block'; document.getElementById('modal').style.display = 'block'; const res = await fetch('api/baseservices/redis/value?type=' + type + '&key=' + encodeURIComponent(key)); const data = await res.json(); this.renderModalContent(data); },
       renderModalContent: function(data) {
          let body = '';
          switch (data.type) {
             case 'string': body = '<div class="form-group"><label>Value</label><textarea id="stringValue" rows="5" style="width:100%">' + escapeHtml(data.value) + '</textarea></div><button class="btn-green" onclick="redis.saveStringValue(\'' + data.key + '\')">Save</button>'; break;
             case 'list': let items = data.value.map(item => '<div class="list-item"><span>' + escapeHtml(item) + '</span><button class="btn-sm btn-red" onclick="redis.deleteListItem(\'' + data.key + '\', \'' + escapeHtml(item) + '\')">Delete</button></div>').join(''); body = '<div class="form-group"><input type="text" id="newListItem" placeholder="New Item" style="width:100%"><button class="btn-green" style="margin-top:10px;" onclick="redis.addListItem(\'' + data.key + '\')">Add</button></div><hr>' + items; break;
             case 'hash': let fields = Object.entries(data.value).map(([f, v]) => '<div class="hash-item"><span><strong>' + escapeHtml(f) + ':</strong> ' + escapeHtml(v) + '</span><button class="btn-sm btn-red" onclick="redis.deleteHashField(\'' + data.key + '\', \'' + escapeHtml(f) + '\')">Delete</button></div>').join(''); body = '<div class="form-group"><input type="text" id="newHashField" placeholder="Field" style="width:100%"><textarea id="newHashValue" placeholder="Value" style="width:100%"></textarea><button class="btn-green" style="margin-top:10px;" onclick="redis.addHashField(\'' + data.key + '\')">Save</button></div><hr>' + fields; break;
             default: body = '<p>Unsupported type: ' + data.type + '</p>';
          }
          document.getElementById('modal-body').innerHTML = body;
       },
       saveStringValue: async function(key) { const value = document.getElementById('stringValue').value; await fetch('api/baseservices/redis/value?type=string&key=' + encodeURIComponent(key), { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ value }) }); this.hideModal(); },
       addListItem: async function(key) { const value = document.getElementById('newListItem').value; if (!value) return; await fetch('api/baseservices/redis/value?type=list&key=' + encodeURIComponent(key), { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ value }) }); this.viewEditKey(key, 'list'); },
       deleteListItem: async function(key, value) { await fetch('api/baseservices/redis/value?type=list&key=' + encodeURIComponent(key) + '&value=' + encodeURIComponent(value), { method: 'DELETE' }); this.viewEditKey(key, 'list'); },
       addHashField: async function(key) { const field = document.getElementById('newHashField').value; const value = document.getElementById('newHashValue').value; if (!field) return; await fetch('api/baseservices/redis/value?type=hash&key=' + encodeURIComponent(key), { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ field, value }) }); this.viewEditKey(key, 'hash'); },
       deleteHashField: async function(key, field) { await fetch('api/baseservices/redis/value?type=hash&key=' + encodeURIComponent(key) + '&field=' + encodeURIComponent(field), { method: 'DELETE' }); this.viewEditKey(key, 'hash'); },
       hideModal: function() { document.getElementById('modal-backdrop').style.display = 'none'; document.getElementById('modal').style.display = 'none'; }
    };
    document.getElementById('modal-close-btn').addEventListener('click', () => redis.hideModal());
    document.getElementById('modal-cancel-btn').addEventListener('click', () => redis.hideModal());
    document.getElementById('modal-backdrop').addEventListener('click', () => redis.hideModal());

    const mysql = {
       currentDB: 'mdm', initialized: false, charts: {},
       init: function() {
          if(this.initialized) return;
          this.charts.metric = new Chart(document.getElementById('mysql-metricChart').getContext('2d'), { type: 'line', data: { labels: [], datasets: [{ label: 'Threads', data: [], borderColor: '#2980b9', fill: false }, { label: 'QPS', data: [], borderColor: '#27ae60', fill: false }] }, options: { responsive: true, animation: false } });
          this.charts.size = new Chart(document.getElementById('mysql-tableSizeChart').getContext('2d'), { type: 'bar', data: { labels: [], datasets: [{ label: 'Size MB', data: [], backgroundColor: 'rgba(52, 152, 219, 0.6)' }] }, options: { responsive: true, indexAxis: 'y' } });
          this.charts.ops = new Chart(document.getElementById('mysql-tableOpsChart').getContext('2d'), { type: 'bar', data: { labels: [], datasets: [{ label: 'Ops', data: [], backgroundColor: 'rgba(231, 76, 60, 0.6)' }] }, options: { responsive: true, indexAxis: 'y' } });
          this.charts.repl = new Chart(document.getElementById('mysql-replChart').getContext('2d'), { type: 'line', data: { labels: [], datasets: [{ label: 'Delay(s)', data: [], borderColor: '#c0392b', fill: false }] }, options: { responsive: true, animation: false } });
          this.loadAll(); setInterval(() => this.loadAll(), 10000); this.initialized = true;
       },
       switchDB: function(db) { this.currentDB = db; this.loadAll(); },
       loadAll: async function() { await Promise.all([ this.loadMetrics(), this.loadTables(), this.loadProcesslist(), this.loadRepl() ]); },
       // [MODIFIED] Added error handling for loadMetrics
       loadMetrics: async function() { 
           try { 
               const res = await fetch('api/baseservices/mysql/metrics/' + this.currentDB); 
               if (!res.ok) throw new Error(await res.text());
               const arr = await res.json(); 
               if (!arr || arr.length === 0) return; 
               const m = arr[0]; 
               document.getElementById('mysql-threads').innerText = m.threads; 
               document.getElementById('mysql-qps').innerText = m.qps; 
               document.getElementById('mysql-connections').innerText = m.max_connections; 
               document.getElementById('mysql-uptime').innerText = m.uptime_str; 
               const now = new Date().toLocaleTimeString(); 
               if (this.charts.metric.data.labels.length > 20) { this.charts.metric.data.labels.shift(); this.charts.metric.data.datasets.forEach(ds => ds.data.shift()); } 
               this.charts.metric.data.labels.push(now); 
               this.charts.metric.data.datasets[0].data.push(m.threads); 
               this.charts.metric.data.datasets[1].data.push(m.qps); 
               this.charts.metric.update(); 
           } catch (e) { 
               console.error('mysql.loadMetrics', e); 
               document.getElementById('mysql-threads').innerText = 'Err';
               document.getElementById('mysql-qps').innerText = 'Err';
           } 
       },
       // [MODIFIED] Added error handling for loadTables
       loadTables: async function() { 
           try { 
               const res = await fetch('api/baseservices/mysql/tables/' + this.currentDB); 
               if (!res.ok) throw new Error(await res.text());
               const data = await res.json(); 
               if (!Array.isArray(data)) return; 
               this.charts.size.data.labels = data.map(d => d.name); 
               this.charts.size.data.datasets[0].data = data.map(d => d.size_mb); 
               this.charts.size.update(); 
               this.charts.ops.data.labels = data.map(d => d.name); 
               this.charts.ops.data.datasets[0].data = data.map(d => d.ops); 
               this.charts.ops.update(); 
           } catch (e) { 
               console.error('mysql.loadTables', e); 
           } 
       },
       // [MODIFIED] Added error handling for loadProcesslist
       loadProcesslist: async function() { 
           try { 
               const res = await fetch('api/baseservices/mysql/processlist/' + this.currentDB); 
               if (!res.ok) throw new Error(await res.text());
               const data = await res.json(); 
               const filter = document.getElementById('mysql-slowFilter').value.toLowerCase(); 
               const tbody = document.querySelector('#mysql-slowQueryTable tbody'); 
               tbody.innerHTML = ''; 
               (data || []).forEach(q => { 
                   if (filter && (!q.info || !q.info.toLowerCase().includes(filter))) return; 
                   tbody.innerHTML += '<tr><td>' + q.id + '</td><td>' + q.user + '</td><td>' + q.host + '</td><td>' + q.db + '</td><td>' + q.command + '</td><td>' + q.time + '</td><td>' + q.state + '</td><td>' + escapeHtml(q.info) + '</td></tr>'; 
               }); 
           } catch (e) { 
               console.error('mysql.loadProcesslist', e); 
               document.querySelector('#mysql-slowQueryTable tbody').innerHTML = '<tr><td colspan="8" style="text-align:center;color:red;">Connection Failed</td></tr>';
           } 
       },
       // [MODIFIED] Added error handling for loadRepl
       loadRepl: async function() { 
           try { 
               const res = await fetch('api/baseservices/mysql/replstatus/' + this.currentDB); 
               if (!res.ok) throw new Error(await res.text());
               const r = await res.json(); 
               document.getElementById('mysql-replStatus').innerHTML = 'Role: ' + r.role + ' | Slave Running: <span class="' + (r.slave_running ? 'pass' : 'fail') + '">' + r.slave_running + '</span> | Delay(s): ' + r.seconds_behind; 
               if (this.charts.repl.data.labels.length > 20) { this.charts.repl.data.labels.shift(); this.charts.repl.data.datasets[0].data.shift(); } 
               this.charts.repl.data.labels.push(new Date().toLocaleTimeString()); 
               this.charts.repl.data.datasets[0].data.push(r.seconds_behind || 0); 
               this.charts.repl.update(); 
           } catch (e) { 
               console.error('mysql.loadRepl', e); 
               document.getElementById('mysql-replStatus').innerHTML = '<span class="fail">Connection Failed</span>';
           } 
       },
       execSQL: async function() { const sql = document.getElementById('mysql-sqlInput').value.trim(); if (!sql) return; const res = await fetch('api/baseservices/mysql/execsql/' + this.currentDB, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ sql }) }); const result = await res.json(); const div = document.getElementById('mysql-sqlResult'); if(result.error) { div.innerHTML = '<div style="color:red; padding:10px;">Error: ' + escapeHtml(result.error) + '</div>'; return; } if(!result.columns || result.columns.length === 0) { div.innerHTML = '<div style="padding:10px; color:#666;">Query executed successfully. No rows returned.</div>'; return; } let tableHtml = '<table class="sql-table"><thead><tr>'; result.columns.forEach(col => { tableHtml += '<th>' + escapeHtml(col) + '</th>'; }); tableHtml += '</tr></thead><tbody>'; if(result.rows) { result.rows.forEach(row => { tableHtml += '<tr>'; row.forEach(cell => { tableHtml += '<td>' + escapeHtml(cell) + '</td>'; }); tableHtml += '</tr>'; }); } tableHtml += '</tbody></table>'; div.innerHTML = tableHtml; }
    };
</script>
</body>
</html>`
