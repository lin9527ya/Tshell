# Rshell Client Batch Build Script
# Used to compile all modified clients and replace them in the server directory

# Set PowerShell encoding to UTF-8 to resolve Chinese garbled characters
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
chcp 65001 | Out-Null

param(
    [switch]$Help
)

# 配置变量
$CLIENT_DIR = "./Rshell-client"
$SERVER_DIR = "./Rshell---A-Cross-Platform-C2/pkg/api/server"
$CLIENTS = @("Reacon_tcp", "Reacon_websocket", "Reacon_kcp", "Reacon_oss", "Reacon_http")

# 支持的平台和架构
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

# Logging functions
function Write-Info {
    param($Message)
    Write-Host "[INFO] $(Get-Date -Format 'HH:mm:ss') - $Message" -ForegroundColor Blue
}

function Write-Success {
    param($Message)
    Write-Host "[SUCCESS] $(Get-Date -Format 'HH:mm:ss') - $Message" -ForegroundColor Green
}

function Write-Warning {
    param($Message)
    Write-Host "[WARNING] $(Get-Date -Format 'HH:mm:ss') - $Message" -ForegroundColor Yellow
}

function Write-Error {
    param($Message)
    Write-Host "[ERROR] $(Get-Date -Format 'HH:mm:ss') - $Message" -ForegroundColor Red
}

function Show-Help {
    @"
Rshell Client Batch Build Script

Usage: .\build-clients.ps1 [parameters]

Parameters:
  -Help          Show this help information

Features:
  - Compile all 5 client types for multiple platforms
  - Automatically replace client files in server directory
  - Support Windows, Linux, macOS multiple architectures

Examples:
  .\build-clients.ps1              # Compile all clients
"@
}

if ($Help) {
    Show-Help
    exit 0
}

# Check Go environment
function Test-GoEnvironment {
    Write-Info "Checking Go environment..."
    
    if (!(Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Error "Go is not installed, please install Go first"
        exit 1
    }
    
    $goVersion = go version
    Write-Info "Go version: $goVersion"
    Write-Success "Go environment check passed"
}

# Compile single client
function Build-Client {
    param(
        [string]$ClientName,
        [hashtable]$Platform
    )
    
    $clientPath = "$CLIENT_DIR/$ClientName"
    $serverTargetDir = "$SERVER_DIR/$($ClientName.Replace('Reacon_', ''))"
    
    if (!(Test-Path $clientPath)) {
        Write-Warning "Client directory does not exist: $clientPath"
        return $false
    }
    
    if (!(Test-Path $serverTargetDir)) {
        Write-Warning "Server target directory does not exist: $serverTargetDir"
        return $false
    }
    
    $outputName = "r_$($Platform.GOOS)_$($Platform.GOARCH)$($Platform.EXT)"
    $outputPath = "$serverTargetDir/$outputName"
    
    Write-Info "Compiling $ClientName for $($Platform.GOOS)/$($Platform.GOARCH)..."
    
    Push-Location $clientPath
    
    try {
        # Set environment variables
        $env:GOOS = $Platform.GOOS
        $env:GOARCH = $Platform.GOARCH
        $env:CGO_ENABLED = "0"
        
        # Compile
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
            } else { 
                0 
            }
            Write-Success "[OK] $outputName ($fileSize MB)"
            return $true
        } else {
            Write-Error "[FAIL] $outputName compilation failed: $result"
            return $false
        }
    }
    catch {
        Write-Error "[FAIL] $outputName compilation exception: $_"
        return $false
    }
    finally {
        Pop-Location
        # Clean up environment variables
        Remove-Item env:GOOS -ErrorAction SilentlyContinue
        Remove-Item env:GOARCH -ErrorAction SilentlyContinue
        Remove-Item env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
}

# Main compilation process
function Start-Build {
    Write-Info "Starting to compile all clients..."
    
    $totalBuilds = $CLIENTS.Count * $PLATFORMS.Count
    $successBuilds = 0
    $failedBuilds = 0
    
    foreach ($client in $CLIENTS) {
        Write-Info "Compiling client: $client"
        
        foreach ($platform in $PLATFORMS) {
            $success = Build-Client -ClientName $client -Platform $platform
            
            if ($success) {
                $successBuilds++
            } else {
                $failedBuilds++
            }
        }
        
        Write-Info "Client $client compilation completed"
        Write-Host ""
    }
    
    Write-Info "Compilation summary:"
    Write-Success "Success: $successBuilds/$totalBuilds"
    if ($failedBuilds -gt 0) {
        Write-Warning "Failed: $failedBuilds/$totalBuilds"
    }
    
    if ($failedBuilds -eq 0) {
        Write-Success "[SUCCESS] All clients compiled successfully!"
    } else {
        Write-Warning "[WARNING] Some clients compilation failed, please check error messages"
    }
}

# Execute compilation
Test-GoEnvironment
Start-Build