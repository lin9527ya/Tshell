@echo off
chcp 65001 >nul
setlocal EnableDelayedExpansion

:: Tshell 服务端 CGO 构建脚本
:: 启用 CGO 以支持 SQLite 数据库

echo ========================================
echo    Tshell 服务端 CGO 构建脚本
echo ========================================
echo.

:: 配置变量
set "SERVER_DIR=Tshell-server"
set "OUTPUT_NAME=tshell-server.exe"

:: 检查服务端目录
if not exist "%SERVER_DIR%" (
    echo [错误] 服务端目录不存在: %SERVER_DIR%
    goto :error
)

echo [信息] 进入服务端目录: %SERVER_DIR%
cd /d "%SERVER_DIR%"

:: 检查 Go 环境
go version >nul 2>&1
if errorlevel 1 (
    echo [错误] Go 未安装或未加入 PATH
    goto :error
)

:: 检查 CGO 依赖
echo [信息] 检查 CGO 环境...
where gcc >nul 2>&1
if errorlevel 1 (
    echo [警告] 未找到 GCC，尝试检查其他编译器...
    where cl >nul 2>&1
    if errorlevel 1 (
        echo [错误] 未找到 C 编译器 (GCC 或 MSVC)
        echo [提示] 请安装以下之一:
        echo         - MinGW-w64 (推荐)
        echo         - TDM-GCC
        echo         - Visual Studio Build Tools
        goto :error
    ) else (
        echo [信息] 找到 MSVC 编译器
    )
) else (
    echo [信息] 找到 GCC 编译器
)

:: 清理旧构建
if exist "%OUTPUT_NAME%" (
    echo [信息] 清理旧的构建文件...
    del "%OUTPUT_NAME%"
)

:: 设置 CGO 环境变量
echo [信息] 配置 CGO 环境变量...
set CGO_ENABLED=1
set GOOS=windows
set GOARCH=amd64

:: 显示环境信息
echo [信息] 构建环境:
echo         CGO_ENABLED=%CGO_ENABLED%
echo         GOOS=%GOOS%
echo         GOARCH=%GOARCH%
echo.

:: 整理依赖
echo [信息] 整理 Go 模块依赖...
go mod tidy
if errorlevel 1 (
    echo [错误] Go 模块依赖整理失败
    goto :error
)

:: 下载依赖
echo [信息] 下载依赖包...
go mod download
if errorlevel 1 (
    echo [错误] 依赖包下载失败
    goto :error
)

:: 构建服务端 (启用 CGO)
echo [信息] 开始构建服务端 (启用 CGO)...
echo [构建] go build -o %OUTPUT_NAME% -ldflags="-s -w" .

go build -o "%OUTPUT_NAME%" -ldflags="-s -w" .
if errorlevel 1 (
    echo [错误] 服务端构建失败
    goto :error
)

:: 检查构建结果
if not exist "%OUTPUT_NAME%" (
    echo [错误] 构建文件不存在: %OUTPUT_NAME%
    goto :error
)

:: 显示构建信息
echo.
echo [成功] 服务端构建完成！
echo [文件] %OUTPUT_NAME%
for %%f in ("%OUTPUT_NAME%") do echo [大小] %%~zf 字节

:: 验证 CGO 编译
echo.
echo [信息] 验证 CGO 编译...
strings "%OUTPUT_NAME%" | findstr "sqlite" >nul
if errorlevel 1 (
    echo [警告] 未检测到 SQLite 相关字符串，可能编译有问题
) else (
    echo [验证] SQLite 支持已正确编译
)

:: 显示启动命令
echo.
echo ========================================
echo            构建完成
echo ========================================
echo.
echo [启动命令] %OUTPUT_NAME% -p 8089
echo [访问地址] http://localhost:8089
echo.
echo [注意事项] 
echo 1. 确保数据库文件存在或程序有创建权限
echo 2. 如需修改端口，使用 -p 参数
echo 3. 配置文件: config.yaml
echo.

goto :end

:error
echo.
echo ========================================
echo            构建失败
echo ========================================
echo.
echo [解决方案]
echo 1. 确保已安装 Go 1.19+
echo 2. 安装 C 编译器 (MinGW-w64 推荐)
echo 3. 检查网络连接以下载依赖
echo 4. 确保有足够的磁盘空间
echo.
exit /b 1

:end
echo [完成] 按任意键退出...
pause >nul