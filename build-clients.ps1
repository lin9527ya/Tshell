param(
    [switch]$Help
)

# Tshell 客户端批量编译 — 协议前缀命名
# 输出格式：{协议前缀}_{GOOS}_{GOARCH}[.exe]
# 例：tcp_linux_amd64、ws_windows_amd64.exe

$CLIENT_DIR = "./Tshell-client"
$SERVER_DIR = "./Tshell-server/pkg/api/server"

# Name=源码目录, Protocol=文件名前缀, Dir=嵌入目录
$CLIENTS = @(
    @{Name="Reacon_tcp"; Protocol="tcp"; Dir="tcp"},
    @{Name="Reacon_tcp_forward"; Protocol="tcp_forward"; Dir="tcp_forward"},
    @{Name="Reacon_websocket"; Protocol="ws"; Dir="websocket"},
    @{Name="Reacon_kcp"; Protocol="kcp"; Dir="kcp"},
    @{Name="Reacon_oss"; Protocol="oss"; Dir="oss"},
    @{Name="Reacon_http"; Protocol="http"; Dir="http"},
    @{Name="Reacon_dns"; Protocol="dns"; Dir="dns"}
)

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

function Write-Err {
    param($Message)
    Write-Host "[ERROR] $Message" -ForegroundColor Red
}

function Show-Help {
    @"
Tshell 客户端批量编译脚本（协议前缀命名）

用法: .\build-clients.ps1 [-Help]

命名规则: {协议前缀}_{os}_{arch}[.exe]
  tcp_linux_amd64
  ws_windows_amd64.exe
  http_linux_amd64
  tcp_forward_windows_amd64.exe

推荐也可使用: .\build-clients-new-naming.ps1  或  .\build-all-clients.ps1
"@
}

if ($Help) {
    Show-Help
    exit 0
}

function Test-GoEnvironment {
    Write-Info "检查Go环境..."
    if (!(Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Err "Go 未安装，请先安装 Go"
        exit 1
    }
    Write-Info "Go版本: $(go version)"
    Write-Success "Go环境检查通过"
}

function Build-Client {
    param(
        [hashtable]$Client,
        [hashtable]$Platform
    )

    $clientPath = "$CLIENT_DIR/$($Client.Name)"
    $serverTargetDir = "$SERVER_DIR/$($Client.Dir)"

    if (!(Test-Path $clientPath)) {
        Write-Warning "客户端目录不存在: $clientPath"
        return $false
    }

    if (!(Test-Path $serverTargetDir)) {
        New-Item -ItemType Directory -Path $serverTargetDir -Force | Out-Null
        Write-Info "已创建目录: $serverTargetDir"
    }

    # 协议前缀命名
    $outputName = "$($Client.Protocol)_$($Platform.GOOS)_$($Platform.GOARCH)$($Platform.EXT)"
    $outputPath = "$serverTargetDir/$outputName"

    Write-Info "编译 $($Client.Name) -> $outputName"

    Push-Location $clientPath
    try {
        $env:GOOS = $Platform.GOOS
        $env:GOARCH = $Platform.GOARCH
        $env:CGO_ENABLED = "0"

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
            } else { 0 }
            Write-Success "✓ $outputName ($fileSize MB)"
            return $true
        } else {
            Write-Err "✗ $outputName 编译失败: $result"
            return $false
        }
    }
    catch {
        Write-Err "✗ $outputName 编译异常: $_"
        return $false
    }
    finally {
        Pop-Location
        Remove-Item env:GOOS -ErrorAction SilentlyContinue
        Remove-Item env:GOARCH -ErrorAction SilentlyContinue
        Remove-Item env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
}

function Start-Build {
    Write-Info "开始编译所有客户端（协议前缀命名）..."

    $totalBuilds = $CLIENTS.Count * $PLATFORMS.Count
    $successBuilds = 0
    $failedBuilds = 0

    foreach ($client in $CLIENTS) {
        Write-Info "编译客户端: $($client.Name) (前缀=$($client.Protocol), 目录=$($client.Dir))"

        foreach ($platform in $PLATFORMS) {
            if (Build-Client -Client $client -Platform $platform) {
                $successBuilds++
            } else {
                $failedBuilds++
            }
        }
        Write-Host ""
    }

    Write-Info "编译总结:"
    Write-Success "成功: $successBuilds/$totalBuilds"
    if ($failedBuilds -gt 0) {
        Write-Warning "失败: $failedBuilds/$totalBuilds"
    } else {
        Write-Success "所有客户端编译完成！"
        Write-Info "示例: tcp_linux_amd64 / ws_windows_amd64.exe / http_linux_amd64"
    }
}

Test-GoEnvironment
Start-Build
