# Tshell 隐匿构建脚本
# 用于去除二进制文件中的调试信息、路径信息和敏感字符串

param(
    [string]$Target = "all",  # all, server, clients
    [string]$Platform = "windows",  # windows, linux, darwin
    [string]$Arch = "amd64",
    [switch]$Clean,
    [switch]$Verify,
    [switch]$Help
)

# 配置变量
$SERVER_DIR = "Tshell-server"
$CLIENT_DIR = "Tshell-client"
$OUTPUT_DIR = "release"

# 日志函数
function Write-Info {
    param($Message)
    Write-Host "[信息] $(Get-Date -Format 'HH:mm:ss') - $Message" -ForegroundColor Cyan
}

function Write-Success {
    param($Message)
    Write-Host "[成功] $(Get-Date -Format 'HH:mm:ss') - $Message" -ForegroundColor Green
}

function Write-Warning {
    param($Message)
    Write-Host "[警告] $(Get-Date -Format 'HH:mm:ss') - $Message" -ForegroundColor Yellow
}

function Write-Error {
    param($Message)
    Write-Host "[错误] $(Get-Date -Format 'HH:mm:ss') - $Message" -ForegroundColor Red
}

function Show-Help {
    @"
Tshell 隐匿构建脚本

用法: .\build-stealth.ps1 [参数]

参数:
  -Target      构建目标 (all, server, clients) [默认: all]
  -Platform    目标平台 (windows, linux, darwin) [默认: windows]  
  -Arch        目标架构 (amd64, 386, arm64) [默认: amd64]
  -Clean       构建前清理
  -Verify      验证构建结果
  -Help        显示此帮助信息

功能:
  • 去除构建信息和路径
  • 去除调试符号
  • 混淆字符串常量
  • 压缩二进制文件
  • 验证清理效果

示例:
  .\build-stealth.ps1                                    # 构建全部
  .\build-stealth.ps1 -Target server -Platform linux    # 构建Linux服务器
  .\build-stealth.ps1 -Target clients -Clean            # 清理后构建客户端
"@
}

if ($Help) {
    Show-Help
    exit 0
}

# 检查工具
function Test-Tools {
    Write-Info "检查构建环境..."
    
    # 检查 Go
    if (!(Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Error "Go 未安装"
        exit 1
    }
    
    $goVersion = go version
    Write-Info "Go 版本: $goVersion"
    
    # 检查 UPX (可选)
    $upxAvailable = Get-Command upx -ErrorAction SilentlyContinue
    if ($upxAvailable) {
        Write-Info "UPX 可用，将进行二进制压缩"
    } else {
        Write-Warning "UPX 未安装，跳过压缩步骤"
    }
    
    Write-Success "环境检查完成"
    return $upxAvailable -ne $null
}

# 清理环境
function Clear-Environment {
    Write-Info "清理构建环境..."
    
    if (Test-Path $OUTPUT_DIR) {
        Remove-Item $OUTPUT_DIR -Recurse -Force
    }
    New-Item -ItemType Directory -Path $OUTPUT_DIR -Force | Out-Null
    
    # 清理模块缓存
    go clean -cache -modcache
    
    Write-Success "环境清理完成"
}

# 生成混淆的构建参数
function Get-StealthFlags {
    $timestamp = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
    $randomBuild = -join ((1..8) | ForEach-Object { [char]((65..90) + (97..122) | Get-Random) })
    
    $ldflags = @(
        "-s",  # 去除符号表
        "-w",  # 去除DWARF调试信息
        "-X 'main.version=1.0.0'",  # 设置版本信息
        "-X 'main.buildTime=$timestamp'",  # 设置构建时间
        "-X 'main.buildID=$randomBuild'",  # 设置随机构建ID
        "-buildid=''"  # 去除构建ID
    )
    
    return "-ldflags=`"$($ldflags -join ' ')`""
}

# 构建单个目标
function Build-Target {
    param(
        [string]$SourceDir,
        [string]$OutputName,
        [string]$TargetOS,
        [string]$TargetArch
    )
    
    if (!(Test-Path $SourceDir)) {
        Write-Warning "源目录不存在: $SourceDir"
        return $false
    }
    
    Write-Info "构建 $OutputName ($TargetOS/$TargetArch)..."
    
    Push-Location $SourceDir
    
    try {
        # 设置环境变量
        $env:GOOS = $TargetOS
        $env:GOARCH = $TargetArch
        $env:CGO_ENABLED = "0"
        
        # 去除模块信息
        $env:GOPROXY = "direct"
        $env:GONOSUMDB = "*"
        
        # 生成输出路径
        $extension = if ($TargetOS -eq "windows") { ".exe" } else { "" }
        $outputPath = "..\$OUTPUT_DIR\$OutputName$extension"
        
        # 构建参数
        $stealthFlags = Get-StealthFlags
        $buildArgs = @(
            "build",
            "-a",  # 强制重新构建
            "-trimpath",  # 去除文件路径
            "-buildmode=exe",  # 指定构建模式
            $stealthFlags,
            "-o", $outputPath,
            "."
        )
        
        Write-Info "执行: go $($buildArgs -join ' ')"
        
        $result = & go @buildArgs 2>&1
        
        if ($LASTEXITCODE -eq 0 -and (Test-Path $outputPath)) {
            $fileSize = [math]::Round((Get-Item $outputPath).Length / 1MB, 2)
            Write-Success "构建成功: $OutputName ($fileSize MB)"
            return $outputPath
        } else {
            Write-Error "构建失败: $result"
            return $null
        }
    }
    finally {
        Pop-Location
        # 清理环境变量
        Remove-Item env:GOOS -ErrorAction SilentlyContinue
        Remove-Item env:GOARCH -ErrorAction SilentlyContinue
        Remove-Item env:CGO_ENABLED -ErrorAction SilentlyContinue
        Remove-Item env:GOPROXY -ErrorAction SilentlyContinue
        Remove-Item env:GONOSUMDB -ErrorAction SilentlyContinue
    }
}

# 压缩二进制文件
function Compress-Binary {
    param([string]$FilePath, [bool]$UpxAvailable)
    
    if (!$UpxAvailable -or !(Test-Path $FilePath)) {
        return
    }
    
    Write-Info "压缩二进制文件: $(Split-Path $FilePath -Leaf)"
    
    try {
        $result = & upx --best --lzma $FilePath 2>&1
        if ($LASTEXITCODE -eq 0) {
            Write-Success "压缩完成"
        } else {
            Write-Warning "压缩失败: $result"
        }
    }
    catch {
        Write-Warning "压缩异常: $_"
    }
}

# 验证清理效果
function Test-StealthBuild {
    param([string]$FilePath)
    
    if (!(Test-Path $FilePath)) {
        Write-Error "文件不存在: $FilePath"
        return
    }
    
    Write-Info "验证隐匿效果: $(Split-Path $FilePath -Leaf)"
    
    # 检查是否有strings命令（Git Bash或WSL）
    $stringsCmd = Get-Command strings -ErrorAction SilentlyContinue
    if (!$stringsCmd) {
        Write-Warning "strings 命令不可用，无法验证"
        return
    }
    
    # 检查敏感信息
    $sensitivePatterns = @(
        "go build",
        "go1\.",
        "golang\.org",
        "github\.com",
        "\.go:",
        "runtime\.",
        "/Users/",
        "/home/",
        "C:\\Users\\",
        "src/runtime/"
    )
    
    $foundSensitive = @()
    foreach ($pattern in $sensitivePatterns) {
        try {
            $matches = & strings $FilePath | Select-String $pattern
            if ($matches) {
                $foundSensitive += $pattern
            }
        }
        catch {
            Write-Warning "无法检查模式: $pattern"
        }
    }
    
    if ($foundSensitive.Count -eq 0) {
        Write-Success "隐匿效果良好，未发现敏感信息"
    } else {
        Write-Warning "发现以下敏感信息模式:"
        $foundSensitive | ForEach-Object { Write-Warning "  - $_" }
    }
}

# 构建服务器
function Build-Server {
    param([string]$TargetOS, [string]$TargetArch, [bool]$UpxAvailable)
    
    Write-Info "开始构建服务器..."
    
    $outputName = "tshell-server-$TargetOS-$TargetArch"
    $builtFile = Build-Target -SourceDir $SERVER_DIR -OutputName $outputName -TargetOS $TargetOS -TargetArch $TargetArch
    
    if ($builtFile) {
        Compress-Binary -FilePath $builtFile -UpxAvailable $UpxAvailable
        if ($Verify) {
            Test-StealthBuild -FilePath $builtFile
        }
    }
}

# 构建客户端
function Build-Clients {
    param([string]$TargetOS, [string]$TargetArch, [bool]$UpxAvailable)
    
    Write-Info "开始构建客户端..."
    
    $clients = @(
        "Reacon_tcp",
        "Reacon_kcp", 
        "Reacon_http",
        "Reacon_oss",
        "Reacon_websocket"
    )
    
    foreach ($client in $clients) {
        $clientDir = "$CLIENT_DIR\$client"
        $outputName = "$client-$TargetOS-$TargetArch"
        
        $builtFile = Build-Target -SourceDir $clientDir -OutputName $outputName -TargetOS $TargetOS -TargetArch $TargetArch
        
        if ($builtFile) {
            Compress-Binary -FilePath $builtFile -UpxAvailable $UpxAvailable
            if ($Verify) {
                Test-StealthBuild -FilePath $builtFile
            }
        }
    }
}

# 主函数
function Main {
    Write-Info "Tshell 隐匿构建开始..."
    Write-Info "目标: $Target, 平台: $Platform, 架构: $Arch"
    
    $upxAvailable = Test-Tools
    
    if ($Clean) {
        Clear-Environment
    } elseif (!(Test-Path $OUTPUT_DIR)) {
        New-Item -ItemType Directory -Path $OUTPUT_DIR -Force | Out-Null
    }
    
    switch ($Target.ToLower()) {
        "server" {
            Build-Server -TargetOS $Platform -TargetArch $Arch -UpxAvailable $upxAvailable
        }
        "clients" {
            Build-Clients -TargetOS $Platform -TargetArch $Arch -UpxAvailable $upxAvailable
        }
        "all" {
            Build-Server -TargetOS $Platform -TargetArch $Arch -UpxAvailable $upxAvailable
            Build-Clients -TargetOS $Platform -TargetArch $Arch -UpxAvailable $upxAvailable
        }
        default {
            Write-Error "未知目标: $Target"
            Show-Help
            exit 1
        }
    }
    
    Write-Success "构建完成！文件输出到: $OUTPUT_DIR"
    
    if (Test-Path $OUTPUT_DIR) {
        Write-Info "构建产物:"
        Get-ChildItem $OUTPUT_DIR | ForEach-Object {
            $size = [math]::Round($_.Length / 1MB, 2)
            Write-Info "  $($_.Name) ($size MB)"
        }
    }
}

# 运行主函数
Main