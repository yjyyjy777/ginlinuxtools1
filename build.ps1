# UEM Deployment Tools 一键打包脚本（支持小版本号自动递增）
$ErrorActionPreference = "Stop"

# 1. 终止可能正在运行的客户端进程
Write-Host ">>> [1/6] 检查并关闭旧进程..." -ForegroundColor Cyan
Stop-Process -Name "uemtools" -Force -ErrorAction SilentlyContinue

# 2. 读取并自增小版本号，获取当前日期
$versionFile = "VERSION"
$majorMinor = "6.2"
$patch = 0

if (Test-Path $versionFile) {
    $currentVer = (Get-Content $versionFile -Raw).Trim()
    if ($currentVer -match "^6\.2\.(\d+)$") {
        $patch = [int]$matches[1] + 1
    }
}
$newVersion = "$majorMinor.$patch"
[System.IO.File]::WriteAllText((Resolve-Path .).Path + "\$versionFile", $newVersion + "`n", [System.Text.Encoding]::UTF8)

$currentDate = Get-Date -Format "yyyy-MM-dd"
Write-Host ">>> [2/6] 更新版本 ($newVersion) 与构建日期 ($currentDate)..." -ForegroundColor Cyan

$htmlPath = "agent/html.go"
if (Test-Path $htmlPath) {
    $content = [System.IO.File]::ReadAllText($htmlPath, [System.Text.Encoding]::UTF8)
    $content = [System.Text.RegularExpressions.Regex]::Replace($content, '<tr><td><strong>版本</strong></td><td>.*?</td></tr>', "<tr><td><strong>版本</strong></td><td>$newVersion</td></tr>")
    $content = [System.Text.RegularExpressions.Regex]::Replace($content, '<tr><td><strong>更新日期</strong></td><td>.*?</td></tr>', "<tr><td><strong>更新日期</strong></td><td>$currentDate</td></tr>")
    [System.IO.File]::WriteAllText($htmlPath, $content, [System.Text.Encoding]::UTF8)
    Write-Host "  -> agent/html.go 已更新为版本: $newVersion, 日期: $currentDate" -ForegroundColor Green
}

# 3. 交叉编译 Linux Agent
Write-Host ">>> [3/6] 编译 Linux Agent (amd64 / arm64)..." -ForegroundColor Cyan
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -o cncyagent_amd64 ./agent
if ($LASTEXITCODE -ne 0) { throw "编译 cncyagent_amd64 失败" }

$env:GOARCH = "arm64"
go build -o cncyagent_arm64 ./agent
if ($LASTEXITCODE -ne 0) { throw "编译 cncyagent_arm64 失败" }

# 4. 同步 Agent 到各发布目录
Write-Host ">>> [4/6] 同步 Agent 二进制文件..." -ForegroundColor Cyan
New-Item -ItemType Directory -Force -Path "dist/windows/uemtools" | Out-Null
Copy-Item cncyagent_amd64 dist/windows/uemtools/cncyagent_amd64 -Force
Copy-Item cncyagent_arm64 dist/windows/uemtools/cncyagent_arm64 -Force
Copy-Item cncyagent_amd64 dist/windows/cncyagent_amd64 -Force
Copy-Item cncyagent_arm64 dist/windows/cncyagent_arm64 -Force

# 5. 编译 Windows 桌面程序
Write-Host ">>> [5/6] 编译 Windows 桌面端 uemtools.exe..." -ForegroundColor Cyan
rsrc -manifest build/windows/app.manifest -ico build/windows/icon.ico -o rsrc.syso
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -tags desktop,production -ldflags "-s -w -H windowsgui" -o dist/windows/uemtools/uemtools.exe .
if ($LASTEXITCODE -ne 0) { throw "编译 uemtools.exe 失败" }
Copy-Item dist/windows/uemtools/uemtools.exe dist/windows/uemtools.exe -Force

# 6. 同步最新 history.json 并生成 ZIP 压缩发布包
Write-Host ">>> [6/6] 同步最新 history.json 并生成发布压缩包 dist/windows/uemtools.zip..." -ForegroundColor Cyan
$histOuter = "dist/windows/history.json"
$histInner = "dist/windows/uemtools/history.json"
if ((Test-Path $histOuter) -and (Test-Path $histInner)) {
    if ((Get-Item $histOuter).LastWriteTime -ge (Get-Item $histInner).LastWriteTime) {
        Copy-Item $histOuter $histInner -Force
    } else {
        Copy-Item $histInner $histOuter -Force
    }
} elseif (Test-Path $histOuter) {
    Copy-Item $histOuter $histInner -Force
}
Compress-Archive -Path dist\windows\uemtools\* -DestinationPath dist\windows\uemtools.zip -Force

Write-Host "==========================================" -ForegroundColor Green
Write-Host "  打包成功完成！版本: $newVersion, 日期: $currentDate" -ForegroundColor Green
Write-Host "==========================================" -ForegroundColor Green
Get-Item dist/windows/uemtools.zip, dist/windows/uemtools/uemtools.exe | Select-Object Name, Length, LastWriteTime
