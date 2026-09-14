# 简化的客户端编译脚本 - 修复中文编码问题
# 设置PowerShell编码为UTF-8解决中文乱码
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

Write-Host "=== Rshell 客户端批量编译 ===" -ForegroundColor Cyan

# 配置
$CLIENT_DIR = "./Rshell-client"
$SERVER_DIR = "./Rshell---A-Cross-Platform-C2/pkg/api/server"
$CLIENTS = @("Reacon_tcp", "Reacon_websocket", "Reacon_kcp", "Reacon_oss", "Reacon_http")

# 主要平台
$PLATFORMS = @(
    @{GOOS="windows"; GOARCH="amd64"; EXT=".exe"},
    @{GOOS="linux"; GOARCH="amd64"; EXT=""}
)

function Write-Info {
    param($Message)
    Write-Host "[INFO] $Message" -ForegroundColor Blue
}

function Write-Success {
    param($Message) 
    Write-Host "[SUCCESS] $Message" -ForegroundColor Green
}

function Write-Error {
    param($Message)
    Write-Host "[ERROR] $Message" -ForegroundColor Red
}

# 检查Go环境
Write-Info "检查Go环境..."
if (!(Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error "Go 未安装，请先安装 Go"
    exit 1
}
$goVersion = go version
Write-Info "Go版本: $goVersion"

# 编译函数
function Build-Client {
    param(
        [string]$ClientName,
        [hashtable]$Platform
    )
    
    $clientPath = "$CLIENT_DIR/$ClientName"
    $serverTargetDir = "$SERVER_DIR/$($ClientName.Replace('Reacon_', ''))"
    
    if (!(Test-Path $clientPath)) {
        Write-Error "客户端目录不存在: $clientPath"
        return $false
    }
    
    if (!(Test-Path $serverTargetDir)) {
        Write-Error "服务端目标目录不存在: $serverTargetDir"
        return $false
    }
    
    $outputName = "r_$($Platform.GOOS)_$($Platform.GOARCH)$($Platform.EXT)"
    $outputPath = "$serverTargetDir/$outputName"
    
    Write-Info "编译 $ClientName for $($Platform.GOOS)/$($Platform.GOARCH)..."
    
    Push-Location $clientPath
    
    try {
        # 设置环境变量
        $env:GOOS = $Platform.GOOS
        $env:GOARCH = $Platform.GOARCH
        $env:CGO_ENABLED = "0"
        
        # 编译
        $result = & go build -a -ldflags "-w -s" -o $outputPath . 2>&1
        
        if ($LASTEXITCODE -eq 0) {
            $fileSize = if (Test-Path $outputPath) { 
                [math]::Round((Get-Item $outputPath).Length / 1MB, 2) 
            } else { 
                0 
            }
            Write-Success "编译成功: $outputName ($fileSize MB)"
            return $true
        } else {
            Write-Error "编译失败: $outputName - $result"
            return $false
        }
    }
    catch {
        Write-Error "编译异常: $outputName - $_"
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

# 主编译流程
Write-Info "开始编译所有客户端..."

$totalBuilds = $CLIENTS.Count * $PLATFORMS.Count
$successBuilds = 0
$failedBuilds = 0

foreach ($client in $CLIENTS) {
    Write-Info "编译客户端: $client"
    
    foreach ($platform in $PLATFORMS) {
        $success = Build-Client -ClientName $client -Platform $platform
        
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
    Write-Error "失败: $failedBuilds/$totalBuilds"
    Write-Host "部分客户端编译失败，请检查错误信息" -ForegroundColor Yellow
} else {
    Write-Host "所有客户端编译完成！" -ForegroundColor Green
}