# Rshell 前端自动构建和部署脚本 (Windows PowerShell 版本)
# 用于将 Vue 3 + Vite 前端构建产物自动集成到 Go 后端

param(
    [switch]$Clean,
    [switch]$NoDeps,
    [switch]$NoBackend,
    [switch]$NoOptimize,
    [switch]$Help
)

# 配置变量
$FRONTEND_DIR = \"./Rshell-web\"
$BACKEND_DIR = \"./Rshell---A-Cross-Platform-C2\"
$DIST_DIR = \"$FRONTEND_DIR/dist\"
$EMBED_DIR = \"$BACKEND_DIR/dist\"
$GO_EMBED_FILE = \"$BACKEND_DIR/main.go\"

# 日志函数
function Write-Info {
    param($Message)
    Write-Host \"[INFO] $Message\" -ForegroundColor Blue
}

function Write-Success {
    param($Message)
    Write-Host \"[SUCCESS] $Message\" -ForegroundColor Green
}

function Write-Warning {
    param($Message)
    Write-Host \"[WARNING] $Message\" -ForegroundColor Yellow
}

function Write-Error {
    param($Message)
    Write-Host \"[ERROR] $Message\" -ForegroundColor Red
}

# 显示帮助信息
function Show-Help {
@"
Rshell 自动构建脚本 (Windows 版本)

用法: .\build-and-deploy.ps1 [参数]

参数:
  -Clean         清理构建缓存后构建
  -NoDeps        跳过依赖安装
  -NoBackend     仅构建前端，不构建后端
  -NoOptimize    跳过构建产物优化
  -Help          显示此帮助信息

示例:
  .\build-and-deploy.ps1              # 完整构建流程
  .\build-and-deploy.ps1 -Clean       # 清理缓存后构建
  .\build-and-deploy.ps1 -NoBackend   # 仅构建前端
"@
}

# 检查依赖
function Test-Dependencies {
    Write-Info \"检查构建依赖...\"
    
    # 检查 Node.js 和 npm
    if (!(Get-Command node -ErrorAction SilentlyContinue)) {
        Write-Error \"Node.js 未安装，请先安装 Node.js\"
        exit 1
    }
    
    if (!(Get-Command npm -ErrorAction SilentlyContinue)) {
        Write-Error \"npm 未安装，请先安装 npm\"
        exit 1
    }
    
    # 检查 Go
    if (!(Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Error \"Go 未安装，请先安装 Go\"
        exit 1
    }
    
    # 检查目录结构
    if (!(Test-Path $FRONTEND_DIR)) {
        Write-Error \"前端目录不存在: $FRONTEND_DIR\"
        exit 1
    }
    
    if (!(Test-Path $BACKEND_DIR)) {
        Write-Error \"后端目录不存在: $BACKEND_DIR\"
        exit 1
    }
    
    Write-Success \"依赖检查通过\"
}

# 安装前端依赖
function Install-FrontendDeps {
    Write-Info \"安装前端依赖...\"
    
    Push-Location $FRONTEND_DIR
    
    try {
        if (Test-Path \"package-lock.json\") {
            npm ci --silent
        } else {
            npm install --silent
        }
        Write-Success \"前端依赖安装完成\"
    }
    catch {
        Write-Error \"前端依赖安装失败: $_\"
        Pop-Location
        exit 1
    }
    finally {
        Pop-Location
    }
}

# 构建前端
function Build-Frontend {
    Write-Info \"开始构建前端应用 (Vue 3 + Vite)...\"
    
    Push-Location $FRONTEND_DIR
    
    try {
        # 清理旧的构建文件
        if (Test-Path \"dist\") {
            Remove-Item -Path \"dist\" -Recurse -Force
            Write-Info \"清理旧的构建文件\"
        }
        
        # 执行构建
        npm run build
        
        # 检查构建结果
        if (!(Test-Path \"dist\")) {
            Write-Error \"前端构建失败，dist 目录不存在\"
            Pop-Location
            exit 1
        }
        
        # 显示构建文件大小
        Write-Info \"构建产物:\"
        Get-ChildItem -Path \"dist\" -Recurse | Format-Table Name, Length -AutoSize
        
        Write-Success \"前端构建完成\"
    }
    catch {
        Write-Error \"前端构建失败: $_\"
        Pop-Location
        exit 1
    }
    finally {
        Pop-Location
    }
}

# 优化构建产物
function Optimize-Build {
    Write-Info \"优化构建产物...\"
    
    Push-Location $DIST_DIR
    
    try {
        # 生成资源清单
        Get-ChildItem -Path \".\" -Recurse -Include \"*.js\", \"*.css\", \"*.html\" | 
            Select-Object -ExpandProperty Name | 
            Out-File -FilePath \"../manifest.txt\" -Encoding UTF8
        
        Write-Info \"已生成资源清单\"
        Write-Success \"构建产物优化完成\"
    }
    catch {
        Write-Warning \"构建产物优化过程中出现问题: $_\"
    }
    finally {
        Pop-Location
    }
}

# 更新后端嵌入文件
function Update-BackendEmbed {
    Write-Info \"更新后端嵌入文件...\"
    
    try {
        # 清理旧的嵌入目录
        if (Test-Path $EMBED_DIR) {
            Remove-Item -Path $EMBED_DIR -Recurse -Force
            Write-Info \"清理旧的嵌入目录\"
        }
        
        # 复制前端构建产物到后端
        Copy-Item -Path $DIST_DIR -Destination $EMBED_DIR -Recurse
        Write-Info \"复制前端构建产物到后端目录\"
        
        # 确保 Go embed 指令正确
        $embedContent = Get-Content $GO_EMBED_FILE -Raw
        if ($embedContent -notmatch "//go:embed dist") {
            Write-Warning \"请确保 main.go 文件包含 //go:embed dist 指令\"
        }
        
        Write-Success \"后端嵌入文件更新完成\"
    }
    catch {
        Write-Error \"更新后端嵌入文件失败: $_\"
        exit 1
    }
}

# 构建后端
function Build-Backend {
    Write-Info \"构建后端应用 (Go + Gin)...\"
    
    Push-Location $BACKEND_DIR
    
    try {
        # 整理依赖
        go mod tidy
        
        # 构建二进制文件
        go build -ldflags=\"-s -w\" -o rshell-server.exe .
        
        # 检查构建结果
        if (!(Test-Path \"rshell-server.exe\")) {
            Write-Error \"后端构建失败\"
            Pop-Location
            exit 1
        }
        
        # 显示二进制文件大小
        Write-Info \"后端二进制文件大小:\"
        Get-Item \"rshell-server.exe\" | Format-Table Name, Length -AutoSize
        
        Write-Success \"后端构建完成\"
    }
    catch {
        Write-Error \"后端构建失败: $_\"
        Pop-Location
        exit 1
    }
    finally {
        Pop-Location
    }
}

# 验证构建结果
function Test-Build {
    Write-Info \"验证构建结果...\"
    
    Push-Location $BACKEND_DIR
    
    try {
        # 检查二进制文件是否存在
        if (Test-Path \"rshell-server.exe\") {
            $fileSize = (Get-Item \"rshell-server.exe\").Length
            if ($fileSize -gt 10MB) {
                Write-Success \"静态文件可能已正确嵌入到二进制文件中 (文件大小: $([math]::Round($fileSize/1MB, 2)) MB)\"
            } else {
                Write-Warning \"二进制文件较小，请检查静态文件是否正确嵌入\"
            }
        }
    }
    catch {
        Write-Warning \"验证构建结果时出现问题: $_\"
    }
    finally {
        Pop-Location
    }
}

# 生成版本信息
function New-VersionInfo {
    $versionFile = \"$BACKEND_DIR/version.txt\"
    $buildTime = Get-Date -Format \"yyyy-MM-dd HH:mm:ss\"
    $gitCommit = \"\"
    
    # 获取 Git 提交信息
    if (Get-Command git -ErrorAction SilentlyContinue) {
        try {
            $gitCommit = git rev-parse --short HEAD 2>$null
        }
        catch {
            $gitCommit = \"N/A\"
        }
    }
    
    $versionContent = @\"
构建时间: $buildTime
Git提交: $gitCommit
前端技术栈: Vue 3 + TypeScript + Element Plus + Vite
后端技术栈: Go + Gin + SQLite + JWT
\"@
    
    $versionContent | Out-File -FilePath $versionFile -Encoding UTF8
    Write-Info \"版本信息已生成: $versionFile\"
}

# 清理构建缓存
function Clear-BuildCache {
    Write-Info \"清理构建缓存...\"
    
    try {
        # 清理前端缓存
        Push-Location $FRONTEND_DIR
        
        if (Test-Path \"node_modules/.cache\") {
            Remove-Item -Path \"node_modules/.cache\" -Recurse -Force
        }
        if (Test-Path \".vite\") {
            Remove-Item -Path \".vite\" -Recurse -Force
        }
        
        Pop-Location
        
        # 清理 Go 缓存
        Push-Location $BACKEND_DIR
        go clean -cache
        Pop-Location
        
        Write-Success \"构建缓存清理完成\"
    }
    catch {
        Write-Warning \"清理构建缓存时出现问题: $_\"
    }
}

# 主函数
function Main {
    if ($Help) {
        Show-Help
        return
    }
    
    # 显示开始信息
    Write-Info \"开始 Rshell 自动构建流程...\"
    Write-Info \"前端: Vue 3 + TypeScript + Element Plus + Vite\"
    Write-Info \"后端: Go + Gin + SQLite + JWT\"
    Write-Host \"\"
    
    # 执行构建流程
    Test-Dependencies
    
    if ($Clean) {
        Clear-BuildCache
    }
    
    if (!$NoDeps) {
        Install-FrontendDeps
    }
    
    Build-Frontend
    
    if (!$NoOptimize) {
        Optimize-Build
    }
    
    Update-BackendEmbed
    
    if (!$NoBackend) {
        Build-Backend
        Test-Build
    }
    
    New-VersionInfo
    
    Write-Host \"\"
    Write-Success \"🎉 Rshell 自动构建完成！\"
    
    if (!$NoBackend) {
        Write-Host \"\"
        Write-Info \"启动命令:\"
        Write-Host \"  cd $BACKEND_DIR && .\\rshell-server.exe -p 8089\"
        Write-Host \"  然后访问: http://localhost:8089\"
    }
}

# 脚本入口点
Main