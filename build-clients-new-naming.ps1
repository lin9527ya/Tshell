# Tshell 客户端编译脚本 - 新的命名规则
# 将客户端文件命名从 r_* 改为协议名开头
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

Write-Host "=== Tshell 客户端批量编译 (新命名规则) ===" -ForegroundColor Cyan
Write-Host "将文件名从 r_* 格式改为 协议名_* 格式" -ForegroundColor Yellow

# 配置
$CLIENT_DIR = "./Tshell-client"
$SERVER_DIR = "./Tshell-server/pkg/api/server"

# 客户端配置：Name=源码目录, Protocol=输出文件名前缀, Dir=嵌入目录(默认=Protocol)
# 命名规则：{Protocol}_{GOOS}_{GOARCH}[.exe]
# 示例：tcp_linux_amd64、ws_windows_amd64.exe、http_linux_amd64
$CLIENTS = @(
    @{Name="Reacon_tcp"; Protocol="tcp"; Dir="tcp"},
    @{Name="Reacon_tcp_forward"; Protocol="tcp_forward"; Dir="tcp_forward"},
    # websocket 目录名是 websocket，文件前缀统一为 ws_
    @{Name="Reacon_websocket"; Protocol="ws"; Dir="websocket"},
    @{Name="Reacon_kcp"; Protocol="kcp"; Dir="kcp"},
    @{Name="Reacon_oss"; Protocol="oss"; Dir="oss"},
    @{Name="Reacon_http"; Protocol="http"; Dir="http"},
    @{Name="Reacon_dns"; Protocol="dns"; Dir="dns"}
)

# 支持的平台和架构
$PLATFORMS = @(
    @{GOOS="windows"; GOARCH="amd64"; EXT=".exe"},
    @{GOOS="windows"; GOARCH="386"; EXT=".exe"},
    @{GOOS="linux"; GOARCH="amd64"; EXT=""},
    @{GOOS="linux"; GOARCH="386"; EXT=""},
    @{GOOS="linux"; GOARCH="arm"; EXT=""},
    @{GOOS="linux"; GOARCH="arm64"; EXT=""},
    @{GOOS="linux"; GOARCH="mips"; EXT=""},
    @{GOOS="linux"; GOARCH="mipsle"; EXT=""},
    @{GOOS="linux"; GOARCH="mips64"; EXT=""},
    @{GOOS="linux"; GOARCH="mips64le"; EXT=""},
    @{GOOS="linux"; GOARCH="loong64"; EXT=""},
    @{GOOS="darwin"; GOARCH="amd64"; EXT=""},
    @{GOOS="darwin"; GOARCH="arm64"; EXT=""}
)

function Write-Info {
    param($Message)
    Write-Host "[INFO] $Message" -ForegroundColor Blue
}

function Write-Success {
    param($Message) 
    Write-Host "[SUCCESS] $Message" -ForegroundColor Green
}

function Write-Warning {
    param($Message)
    Write-Host "[WARNING] $Message" -ForegroundColor Yellow
}

function Write-Error {
    param($Message)
    Write-Host "[ERROR] $Message" -ForegroundColor Red
}

# 检查Go环境
function Test-GoEnvironment {
    Write-Info "检查Go环境..."
    
    if (!(Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Error "Go 未安装，请先安装 Go"
        exit 1
    }
    
    $goVersion = go version
    Write-Info "Go版本: $goVersion"
    Write-Success "Go环境检查通过"
}

# 编译单个客户端
function Build-Client {
    param(
        [hashtable]$Client,
        [hashtable]$Platform
    )
    
    $clientPath = "$CLIENT_DIR/$($Client.Name)"
    # Dir=嵌入目录；Protocol=文件名前缀（websocket 目录 / ws 前缀）
    $protocolDir = if ($Client.Dir) { $Client.Dir } else { $Client.Protocol }
    $serverTargetDir = "$SERVER_DIR/$protocolDir"

    if (!(Test-Path $clientPath)) {
        Write-Warning "客户端目录不存在: $clientPath"
        return $false
    }

    if (!(Test-Path $serverTargetDir)) {
        New-Item -ItemType Directory -Path $serverTargetDir -Force | Out-Null
        Write-Info "已创建目录: $serverTargetDir"
    }

    # 命名规则：{协议前缀}_{os}_{arch}[.exe]
    # 例：tcp_linux_amd64、ws_windows_amd64.exe
    $outputName = "$($Client.Protocol)_$($Platform.GOOS)_$($Platform.GOARCH)$($Platform.EXT)"
    $outputPath = "$serverTargetDir/$outputName"
    
    Write-Info "编译 $($Client.Name) -> $outputName"
    
    Push-Location $clientPath
    
    try {
        # 设置环境变量
        $env:GOOS = $Platform.GOOS
        $env:GOARCH = $Platform.GOARCH
        $env:CGO_ENABLED = "0"
        
        # 编译
        $buildArgs = @(
            "build",
            "-a",
            "-ldflags", "-w -s",
            "-o", $outputPath,
            "."
        )
        
        $result = & go @buildArgs 2>&1
        
        if ($LASTEXITCODE -eq 0) {
            $fileSize = if (Test-Path $outputPath) { 
                [math]::Round((Get-Item $outputPath).Length / 1MB, 2) 
            } else { 
                0 
            }
            Write-Success "✓ $outputName ($fileSize MB)"
            return $true
        } else {
            Write-Error "✗ $outputName 编译失败: $result"
            return $false
        }
    }
    catch {
        Write-Error "✗ $outputName 编译异常: $_"
        return $false
    }
    finally {
        Pop-Location
        # 清理环境变量
        Remove-Item env:GOOS -ErrorAction SilentlyContinue
        Remove-Item env:GOARCH -ErrorAction SilentlyContinue
        Remove-Item env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
}

# 清理旧文件（r_* / t_* 历史命名）
function Remove-OldFiles {
    Write-Info "清理旧的客户端文件..."

    foreach ($client in $CLIENTS) {
        $protocolDir = if ($client.Dir) { $client.Dir } else { $client.Protocol }
        $serverTargetDir = "$SERVER_DIR/$protocolDir"

        if (Test-Path $serverTargetDir) {
            Get-ChildItem "$serverTargetDir/r_*","$serverTargetDir/t_*" -ErrorAction SilentlyContinue | ForEach-Object {
                Write-Info "删除旧文件: $($_.Name)"
                Remove-Item $_.FullName -Force
            }
        }
    }

    Write-Success "旧文件清理完成"
}

# 主编译流程
function Start-Build {
    Write-Info "开始编译所有客户端..."
    
    $totalBuilds = $CLIENTS.Count * $PLATFORMS.Count
    $successBuilds = 0
    $failedBuilds = 0
    
    foreach ($client in $CLIENTS) {
        Write-Info "编译客户端: $($client.Name) -> $($client.Protocol)"
        
        foreach ($platform in $PLATFORMS) {
            $success = Build-Client -Client $client -Platform $platform
            
            if ($success) {
                $successBuilds++
            } else {
                $failedBuilds++
            }
        }
        
        Write-Host ""
    }
    
    Write-Host "=== 编译总结 ===" -ForegroundColor Cyan
    Write-Success "成功: $successBuilds/$totalBuilds"
    if ($failedBuilds -gt 0) {
        Write-Warning "失败: $failedBuilds/$totalBuilds"
        Write-Warning "部分客户端编译失败，请检查错误信息"
    } else {
        Write-Success "🎉 所有客户端编译完成！"
        Write-Success "新的命名规则已应用：{协议前缀}_{os}_{arch}[.exe]"
        Write-Info "示例文件名："
        Write-Info "  - tcp_windows_amd64.exe"
        Write-Info "  - tcp_linux_amd64"
        Write-Info "  - ws_windows_amd64.exe"
        Write-Info "  - ws_linux_amd64"
        Write-Info "  - http_linux_amd64"
        Write-Info "  - tcp_forward_windows_amd64.exe"
    }
}

# 执行编译
Test-GoEnvironment
Remove-OldFiles
Start-Build