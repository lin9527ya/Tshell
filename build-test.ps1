
$CLIENT_DIR = "./Tshell-client"
$SERVER_DIR = "./Tshell-server/pkg/api/server"

$CLIENTS = @(
    @{Name="Reacon_tcp"; Protocol="tcp"},
    @{Name="Reacon_tcp_forward"; Protocol="tcp_forward"}, 
    @{Name="Reacon_websocket"; Protocol="websocket"},
    @{Name="Reacon_kcp"; Protocol="kcp"}
)

$PLATFORMS = @(
    @{GOOS="windows"; GOARCH="amd64"; EXT=".exe"},
    @{GOOS="linux"; GOARCH="amd64"; EXT=""}
)

Write-Host "=== 开始编译 ===" -ForegroundColor Cyan

foreach ($client in $CLIENTS) {
    foreach ($platform in $PLATFORMS) {
        $clientPath = "$CLIENT_DIR/$($client.Name)"
        $serverTargetDir = "$SERVER_DIR/$($client.Protocol)"
        
        if ($client.Protocol -eq "tcp_forward") {
            $serverTargetDir = "$SERVER_DIR/tcp_forward"
        }
        
        $outputName = "$($client.Protocol)_$($platform.GOOS)_$($platform.GOARCH)$($platform.EXT)"
        $outputPath = "$serverTargetDir/$outputName"
        
        Write-Host "编译: $($client.Name) -> $outputName" -ForegroundColor Blue
        
        Push-Location $clientPath
        $env:GOOS = $platform.GOOS
        $env:GOARCH = $platform.GOARCH
        $env:CGO_ENABLED = "0"
        
        go build -a -ldflags "-w -s" -o $outputPath .
        
        if ($LASTEXITCODE -eq 0) {
            $fileSize = [math]::Round((Get-Item $outputPath).Length / 1MB, 2)
            Write-Host "成功: $outputName ($fileSize MB)" -ForegroundColor Green
        } else {
            Write-Host "失败: $outputName" -ForegroundColor Red
        }
        
        Pop-Location
        Remove-Item env:GOOS -ErrorAction SilentlyContinue
        Remove-Item env:GOARCH -ErrorAction SilentlyContinue
        Remove-Item env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
}

Write-Host "=== 编译完成 ===" -ForegroundColor Cyan

