# Tshell 客户端编译脚本 - 新命名规则
# 将客户端文件从 r_* 改为协议名_*
param(
    [switch]$CleanOld = $false
)

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

Write-Host "=== Tshell 客户端批量编译 (新命名规则) ===" -ForegroundColor Cyan
Write-Host "目标格式: 协议名_操作系统_架构" -ForegroundColor Yellow
Write-Host ""

# 配置路径
$CLIENT_DIR = ".\Tshell-client"
$SERVER_DIR = ".\Tshell-server\pkg\api\server"

# 客户端配置：Protocol=文件名前缀，ServerDir=嵌入目录
# 命名：{Protocol}_{OS}_{ARCH}[.exe]  例：tcp_linux_amd64、ws_windows_amd64.exe
$CLIENTS = @(
    @{Name="Reacon_tcp"; Protocol="tcp"; ServerDir="tcp"},
    @{Name="Reacon_tcp_forward"; Protocol="tcp_forward"; ServerDir="tcp_forward"},
    @{Name="Reacon_websocket"; Protocol="ws"; ServerDir="websocket"},
    @{Name="Reacon_kcp"; Protocol="kcp"; ServerDir="kcp"},
    @{Name="Reacon_oss"; Protocol="oss"; ServerDir="oss"},
    @{Name="Reacon_http"; Protocol="http"; ServerDir="http"},
    @{Name="Reacon_dns"; Protocol="dns"; ServerDir="dns"}
)

# 编译平台
$PLATFORMS = @(
    @{OS="windows"; ARCH="amd64"; EXT=".exe"},
    @{OS="windows"; ARCH="386"; EXT=".exe"},
    @{OS="linux"; ARCH="amd64"; EXT=""},
    @{OS="linux"; ARCH="386"; EXT=""},
    @{OS="linux"; ARCH="arm"; EXT=""},
    @{OS="linux"; ARCH="arm64"; EXT=""},
    @{OS="linux"; ARCH="mips"; EXT=""},
    @{OS="linux"; ARCH="mipsle"; EXT=""},
    @{OS="linux"; ARCH="mips64"; EXT=""},
    @{OS="linux"; ARCH="mips64le"; EXT=""},
    @{OS="linux"; ARCH="loong64"; EXT=""},
    @{OS="darwin"; ARCH="amd64"; EXT=""},
    @{OS="darwin"; ARCH="arm64"; EXT=""}
)

# 检查Go环境
function Test-GoEnvironment {
    Write-Host "[检查] 验证Go环境..." -ForegroundColor Blue
    
    if (!(Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Host "[错误] Go 未安装" -ForegroundColor Red
        exit 1
    }
    
    $version = go version
    Write-Host "[信息] $version" -ForegroundColor Green
    return $true
}

# 清理旧文件
function Remove-OldFiles {
    if (!$CleanOld) {
        return
    }
    
    Write-Host "[清理] 删除旧的 r_* 文件..." -ForegroundColor Yellow
    
    foreach ($client in $CLIENTS) {
        $targetDir = "$SERVER_DIR\$($client.ServerDir)"
        if (Test-Path $targetDir) {
            Get-ChildItem "$targetDir\r_*" -ErrorAction SilentlyContinue | ForEach-Object {
                Write-Host "  删除: $($_.Name)" -ForegroundColor Gray
                Remove-Item $_.FullName -Force
            }
        }
    }
    Write-Host "[完成] 旧文件清理完成" -ForegroundColor Green
}

# 编译单个客户端
function Build-SingleClient {
    param($Client, $Platform)
    
    $clientPath = "$CLIENT_DIR\$($Client.Name)"
    $serverTargetDir = "$SERVER_DIR\$($Client.ServerDir)"
    
    # 检查路径
    if (!(Test-Path $clientPath)) {
        Write-Host "  [跳过] 客户端目录不存在: $($Client.Name)" -ForegroundColor Yellow
        return $false
    }
    
    if (!(Test-Path $serverTargetDir)) {
        Write-Host "  [创建] 服务端目录: $($Client.ServerDir)" -ForegroundColor Blue
        New-Item -ItemType Directory -Path $serverTargetDir -Force | Out-Null
    }
    
    # 生成文件名
    $fileName = "$($Client.Protocol)_$($Platform.OS)_$($Platform.ARCH)$($Platform.EXT)"
    $outputPath = "$serverTargetDir\$fileName"
    
    # 编译
    Push-Location $clientPath
    try {
        $env:GOOS = $Platform.OS
        $env:GOARCH = $Platform.ARCH
        $env:CGO_ENABLED = "0"
        
        $null = go build -a -ldflags "-w -s" -o $outputPath . 2>&1
        
        if ($LASTEXITCODE -eq 0 -and (Test-Path $outputPath)) {
            $size = [math]::Round((Get-Item $outputPath).Length / 1MB, 1)
            Write-Host "  [成功] $fileName ($size MB)" -ForegroundColor Green
            return $true
        } else {
            Write-Host "  [失败] $fileName" -ForegroundColor Red
            return $false
        }
    }
    finally {
        Pop-Location
        Remove-Item env:GOOS -ErrorAction SilentlyContinue
        Remove-Item env:GOARCH -ErrorAction SilentlyContinue
        Remove-Item env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
}

# 主编译流程
function Start-BuildProcess {
    $totalCount = 0
    $successCount = 0
    
    foreach ($client in $CLIENTS) {
        Write-Host "[编译] $($client.Name) -> $($client.Protocol)" -ForegroundColor Cyan
        
        foreach ($platform in $PLATFORMS) {
            $totalCount++
            if (Build-SingleClient -Client $client -Platform $platform) {
                $successCount++
            }
        }
        Write-Host ""
    }
    
    # 编译总结
    Write-Host "=== 编译总结 ===" -ForegroundColor Cyan
    Write-Host "总计: $totalCount 个文件" -ForegroundColor White
    Write-Host "成功: $successCount 个文件" -ForegroundColor Green
    Write-Host "失败: $($totalCount - $successCount) 个文件" -ForegroundColor Red
    
    if ($successCount -eq $totalCount) {
        Write-Host ""
        Write-Host "所有客户端编译完成！" -ForegroundColor Green
        Write-Host "新命名规则示例: {协议前缀}_{os}_{arch}[.exe]" -ForegroundColor Yellow
        Write-Host "  tcp_windows_amd64.exe" -ForegroundColor Gray
        Write-Host "  tcp_linux_amd64" -ForegroundColor Gray
        Write-Host "  ws_windows_amd64.exe" -ForegroundColor Gray
        Write-Host "  ws_linux_amd64" -ForegroundColor Gray
        Write-Host "  http_linux_amd64" -ForegroundColor Gray
    }
}

# 执行编译
try {
    Test-GoEnvironment
    Remove-OldFiles
    Start-BuildProcess
}
catch {
    Write-Host "[错误] 编译过程中发生异常: $_" -ForegroundColor Red
    exit 1
}
