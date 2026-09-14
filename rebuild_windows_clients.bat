@echo off
echo ================================
echo 重新编译Windows客户端版本
echo ================================

echo.
echo [1/5] 编译 Reacon_tcp Windows版本...
cd "c:\Users\admin\Desktop\Tshell\Tshell-client\Reacon_tcp"
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o r_windows_amd64.exe .
if errorlevel 1 (
    echo ❌ Reacon_tcp 编译失败
    goto :error
) else (
    echo ✅ Reacon_tcp 编译成功
)

echo.
echo [2/5] 编译 Reacon_kcp windows版本...
cd "c:\Users\admin\Desktop\Tshell\Tshell-client\Reacon_kcp"
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o r_windows_amd64.exe .
if errorlevel 1 (
    echo ❌ Reacon_kcp 编译失败
    goto :error
) else (
    echo ✅ Reacon_kcp 编译成功
)

echo.
echo [3/5] 编译 Reacon_http windows版本...
cd "c:\Users\admin\Desktop\Tshell\Tshell-client\Reacon_http"
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o r_windows_amd64.exe .
if errorlevel 1 (
    echo ❌ Reacon_http 编译失败
    goto :error
) else (
    echo ✅ Reacon_http 编译成功
)

echo.
echo [4/5] 编译 Reacon_oss windows版本...
cd "c:\Users\admin\Desktop\Tshell\Tshell-client\Reacon_oss"
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o r_windows_amd64.exe .
if errorlevel 1 (
    echo ❌ Reacon_oss 编译失败
    goto :error
) else (
    echo ✅ Reacon_oss 编译成功
)

echo.
echo [5/5] 编译 Reacon_websocket windows版本...
cd "c:\Users\admin\Desktop\Tshell\Tshell-client\Reacon_websocket"
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o r_windows_amd64.exe .
if errorlevel 1 (
    echo ❌ Reacon_websocket 编译失败
    goto :error
) else (
    echo ✅ Reacon_websocket 编译成功
)

echo.
echo ================================
echo 🎉 所有Windows客户端编译完成！
echo ================================
echo.
echo 编译产物位置：
echo - Reacon_tcp_linux: c:\Users\admin\Desktop\Tshell\Tshell-client\Reacon_tcp\r_windows_amd.exe
echo - Reacon_kcp_linux: c:\Users\admin\Desktop\Tshell\Tshell-client\Reacon_kcp\r_windows_amd.exe
echo - Reacon_http_linux: c:\Users\admin\Desktop\Tshell\Tshell-client\Reacon_http\r_windows_amd.exe
echo - Reacon_oss_linux: c:\Users\admin\Desktop\Tshell\Tshell-client\Reacon_oss\r_windows_amd.exe
echo - Reacon_websocket_linux: c:\Users\admin\Desktop\Tshell\Tshell-client\Reacon_websocket\r_windows_amd.exe
echo.
echo 请将对应的客户端上传到您的Windows服务器并重新运行
goto :end

:error
echo.
echo ❌ 编译过程中出现错误，请检查Go环境和代码
pause
exit /b 1

:end
pause