# Rshell 一键构建部署脚本 (Windows PowerShell)
# 版本: 1.0.0
# 支持平台: Windows

param(
    [switch]$FrontendOnly,
    [switch]$BackendOnly, 
    [switch]$Clean,
    [switch]$Help
)

# 配置
$FrontendDir = "Rshell-web"
$BackendDir = "Rshell---A-Cross-Platform-C2"
$ServerName = "rshell-server.exe"

# 颜色函数
function Write-ColorOutput($ForegroundColor) {
    $fc = $host.UI.RawUI.ForegroundColor
    $host.UI.RawUI.ForegroundColor = $ForegroundColor
    if ($args) {
        Write-Output $args
    }
    else {
        $input | Write-Output
    }
    $host.UI.RawUI.ForegroundColor = $fc
}

function Write-Status($Message) {
    Write-ColorOutput Blue "[INFO] $Message"
}

function Write-Success($Message) {
    Write-ColorOutput Green "[SUCCESS] $Message"
}

function Write-Warning($Message) {
    Write-ColorOutput Yellow "[WARNING] $Message"
}

function Write-Error($Message) {
    Write-ColorOutput Red "[ERROR] $Message"
}

function Write-Header($Message) {
    Write-ColorOutput Magenta "================================"
    Write-ColorOutput Magenta $Message
    Write-ColorOutput Magenta "================================"
}

# 检查命令是否存在
function Test-Command($Command) {
    try {
        Get-Command $Command -ErrorAction Stop | Out-Null
        return $true
    }
    catch {
        return $false
    }
}

# 检查环境
function Test-Environment {
    Write-Header "环境检查"
    
    $errors = 0
    
    # 检查Node.js
    if (Test-Command "node") {
        $nodeVersion = node --version
        Write-Success "Node.js: $nodeVersion"
        
        # 检查版本是否>=16
        $majorVersion = [int]($nodeVersion -replace 'v(\d+)\..*', '$1')
        if ($majorVersion -lt 16) {
            Write-Warning "建议使用Node.js 16或更高版本"
        }
    }
    else {
        Write-Error "Node.js 未安装或不在PATH中"
        $errors++
    }
    
    # 检查npm
    if (Test-Command "npm") {
        $npmVersion = npm --version
        Write-Success "npm: v$npmVersion"
    }
    else {
        Write-Error "npm 未安装或不在PATH中"
        $errors++
    }
    
    # 检查Go
    if (Test-Command "go") {
        $goVersion = go version
        Write-Success "Go: $goVersion"
    }
    else {
        Write-Error "Go 未安装或不在PATH中"
        $errors++
    }
    
    if ($errors -gt 0) {
        Write-Error "环境检查失败，请安装缺失的依赖"
        exit 1
    }
    
    Write-Success "环境检查通过"
}

# 构建前端
function Build-Frontend {
    Write-Header "构建前端应用"
    
    if (!(Test-Path $FrontendDir)) {
        Write-Error "前端目录不存在: $FrontendDir"
        exit 1
    }
    
    Set-Location $FrontendDir
    
    Write-Status "检查package.json..."
    if (!(Test-Path "package.json")) {
        Write-Error "package.json不存在"
        exit 1
    }
    
    Write-Status "安装依赖..."
    try {
        $null = npm ci --silent 2>$null
        if ($LASTEXITCODE -ne 0) {
            $null = npm install --silent
            if ($LASTEXITCODE -ne 0) {
                throw "依赖安装失败"
            }
        }
        Write-Success "依赖安装完成"
    }
    catch {
        Write-Error "依赖安装失败: $_"
        exit 1
    }
    
    Write-Status "构建前端应用..."
    try {
        npm run build
        if ($LASTEXITCODE -ne 0) {
            throw "前端构建失败"
        }
        Write-Success "前端构建完成"
    }
    catch {
        Write-Error "前端构建失败: $_"
        exit 1
    }
    
    Set-Location ..
}

# 复制到后端
function Copy-ToBackend {
    Write-Header "复制构建产物到后端"
    
    if (!(Test-Path $BackendDir)) {
        Write-Error "后端目录不存在: $BackendDir"
        exit 1
    }
    
    Set-Location $FrontendDir
    
    if (Test-Path "scripts/copy-to-backend.js") {
        Write-Status "使用Node.js脚本复制文件..."
        try {
            node scripts/copy-to-backend.js
            if ($LASTEXITCODE -ne 0) {
                throw "文件复制失败"
            }
            Write-Success "文件复制完成"
        }
        catch {
            Write-Error "文件复制失败: $_"
            exit 1
        }
    }
    else {
        # 手动复制
        Write-Status "手动复制文件..."
        
        if (!(Test-Path "dist")) {
            Write-Error "构建输出目录不存在"
            exit 1
        }
        
        # 创建目标目录
        $targetDir = "..\$BackendDir\dist"
        if (!(Test-Path $targetDir)) {
            New-Item -ItemType Directory -Path $targetDir -Force | Out-Null
        }
        
        # 复制文件
        Copy-Item -Path "dist\*" -Destination $targetDir -Recurse -Force
        Write-Success "文件复制完成"
    }
    
    Set-Location ..
}

# 构建后端
function Build-Backend {
    Write-Header "编译Go后端服务器"
    
    Set-Location $BackendDir
    
    Write-Status "检查Go模块..."
    if (!(Test-Path "go.mod")) {
        Write-Error "go.mod不存在"
        exit 1
    }
    
    Write-Status "下载Go依赖..."
    try {
        go mod download
        if ($LASTEXITCODE -ne 0) {
            Write-Warning "Go依赖下载失败，继续尝试编译..."
        }
        else {
            Write-Success "Go依赖下载完成"
        }
    }
    catch {
        Write-Warning "Go依赖下载失败，继续尝试编译..."
    }
    
    Write-Status "编译服务器..."
    try {
        $env:CGO_ENABLED = "0"
        go build -ldflags="-w -s" -trimpath -o $ServerName .
        if ($LASTEXITCODE -ne 0) {
            throw "服务器编译失败"
        }
        Write-Success "服务器编译完成: $ServerName"
        
        # 显示文件信息
        if (Test-Path $ServerName) {
            $fileSize = (Get-Item $ServerName).Length / 1MB
            Write-Status "服务器文件大小: $([math]::Round($fileSize, 2)) MB"
        }
    }
    catch {
        Write-Error "服务器编译失败: $_"
        exit 1
    }
    
    Set-Location ..
}

# 验证构建结果
function Test-BuildResult {
    Write-Header "验证构建结果"
    
    # 检查后端可执行文件
    if (Test-Path "$BackendDir\$ServerName") {
        Write-Success "后端服务器存在"
    }
    else {
        Write-Error "后端服务器不存在"
        return $false
    }
    
    # 检查前端文件
    if (Test-Path "$BackendDir\dist\index.html") {
        Write-Success "前端主页存在"
    }
    else {
        Write-Error "前端主页不存在"
        return $false
    }
    
    if (Test-Path "$BackendDir\dist\static") {
        $staticFiles = Get-ChildItem -Path "$BackendDir\dist\static" -Recurse -File
        Write-Success "静态资源: $($staticFiles.Count) 个文件"
    }
    else {
        Write-Warning "静态资源目录不存在"
    }
    
    Write-Success "构建验证通过"
    return $true
}

# 显示使用说明
function Show-Usage {
    Write-Header "构建完成 - 使用说明"
    
    Write-Host "服务器位置: " -NoNewline
    Write-ColorOutput Cyan "$BackendDir\$ServerName"
    Write-Host ""
    Write-Host "启动服务器:"
    Write-Host "  " -NoNewline
    Write-ColorOutput Cyan "cd $BackendDir"
    Write-Host "  " -NoNewline  
    Write-ColorOutput Cyan ".\$ServerName"
    Write-Host ""
    Write-Host "或指定端口:"
    Write-Host "  " -NoNewline
    Write-ColorOutput Cyan ".\$ServerName -p 8080"
    Write-Host ""
    Write-Host "或指定配置文件:"
    Write-Host "  " -NoNewline
    Write-ColorOutput Cyan ".\$ServerName -config config.yaml"
    Write-Host ""
    Write-Host "构建脚本使用:"
    Write-Host "  " -NoNewline
    Write-ColorOutput Cyan ".\quick-build.ps1"
    Write-Host "                   # 完整构建"
    Write-Host "  " -NoNewline
    Write-ColorOutput Cyan ".\quick-build.ps1 -FrontendOnly"
    Write-Host "  # 仅构建前端"
    Write-Host "  " -NoNewline
    Write-ColorOutput Cyan ".\quick-build.ps1 -BackendOnly"
    Write-Host "   # 仅编译后端"
    Write-Host "  " -NoNewline
    Write-ColorOutput Cyan ".\quick-build.ps1 -Clean"
    Write-Host "         # 清理后构建"
    Write-Host ""
}

# 清理函数
function Invoke-Cleanup {
    Write-Status "清理临时文件..."
    
    # 清理前端缓存
    if (Test-Path "$FrontendDir\node_modules\.vite") {
        Remove-Item -Path "$FrontendDir\node_modules\.vite" -Recurse -Force
    }
    
    # 清理前端构建产物
    if (Test-Path "$FrontendDir\dist") {
        Remove-Item -Path "$FrontendDir\dist" -Recurse -Force
    }
}

# 主函数
function Main {
    Write-Header "Rshell 自动构建部署工具 v1.0.0 (Windows)"
    
    if ($Help) {
        Write-Host "用法: .\quick-build.ps1 [选项]"
        Write-Host ""
        Write-Host "选项:"
        Write-Host "  -FrontendOnly    仅构建前端"
        Write-Host "  -BackendOnly     仅编译后端"
        Write-Host "  -Clean           清理后构建"
        Write-Host "  -Help            显示帮助信息"
        Write-Host ""
        exit 0
    }
    
    # 记录开始时间
    $startTime = Get-Date
    
    try {
        # 清理构建
        if ($Clean) {
            Invoke-Cleanup
        }
        
        # 执行构建流程
        if (!$BackendOnly) {
            Test-Environment
            Build-Frontend
            Copy-ToBackend
        }
        
        if (!$FrontendOnly) {
            Build-Backend
        }
        
        if (!$FrontendOnly -and !$BackendOnly) {
            $null = Test-BuildResult
        }
        
        # 计算耗时
        $endTime = Get-Date
        $duration = ($endTime - $startTime).TotalSeconds
        
        Write-Success "构建完成! 耗时: $([math]::Round($duration, 1))s"
        
        if (!$FrontendOnly -and !$BackendOnly) {
            Show-Usage
        }
    }
    catch {
        Write-Error "构建过程中发生错误: $_"
        exit 1
    }
}

# 运行主函数
Main