# Tshell服务器CGO编译脚本
# 解决SQLite CGO编译问题

Write-Host "🔧 开始CGO编译Tshell服务器..." -ForegroundColor Cyan

# 设置编译环境
$env:CGO_ENABLED = "1"
$env:PATH = $env:PATH + ";C:\TDM-GCC-64\bin"

# 检查GCC是否可用
Write-Host "📋 检查编译环境..." -ForegroundColor Yellow
try {
    $gccVersion = & "C:\TDM-GCC-64\bin\gcc.exe" --version 2>$null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✅ GCC编译器已就绪" -ForegroundColor Green
    } else {
        throw "GCC测试失败"
    }
} catch {
    Write-Host "❌ GCC编译器不可用" -ForegroundColor Red
    Write-Host "请确保TDM-GCC已正确安装" -ForegroundColor Yellow
    exit 1
}

# 切换到服务器目录
Set-Location "c:\Users\microuser\Desktop\Tshell\Tshell-server"

# 下载依赖
Write-Host "📦 下载Go模块依赖..." -ForegroundColor Yellow
go mod tidy

# 编译服务器
Write-Host "🔨 编译Tshell服务器 (启用CGO)..." -ForegroundColor Yellow
$buildResult = & "C:\TDM-GCC-64\bin\gcc.exe" --version; go build -o tshell-server-cgo.exe main.go

if ($LASTEXITCODE -eq 0) {
    Write-Host "✅ 编译成功！" -ForegroundColor Green
    
    # 检查文件是否生成
    if (Test-Path "tshell-server-cgo.exe") {
        $fileSize = (Get-Item "tshell-server-cgo.exe").Length / 1MB
        Write-Host "📁 生成文件: tshell-server-cgo.exe (${fileSize:F1} MB)" -ForegroundColor Cyan
        
        Write-Host ""
        Write-Host "🎉 CGO编译完成！" -ForegroundColor Green
        Write-Host "💡 现在可以运行: .\tshell-server-cgo.exe" -ForegroundColor Cyan
        Write-Host "🌐 Web界面将在: http://127.0.0.1:8089" -ForegroundColor Cyan
    } else {
        Write-Host "⚠️  编译完成但未找到输出文件" -ForegroundColor Yellow
    }
} else {
    Write-Host "❌ 编译失败" -ForegroundColor Red
    Write-Host "请检查错误信息并重试" -ForegroundColor Yellow
    exit 1
}