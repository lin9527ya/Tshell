#!/usr/bin/env powershell

# 前端API调用诊断脚本

Write-Host "API调用诊断脚本" -ForegroundColor Green

$serverUrl = "http://localhost:8089"

# 步骤1: 登录
Write-Host "步骤1: 登录测试" -ForegroundColor Yellow

try {
    $loginData = @{
        username = "admin"
        password = "123456"
    } | ConvertTo-Json

    $headers = @{
        "Content-Type" = "application/json"
    }

    $loginResponse = Invoke-RestMethod -Uri "$serverUrl/api/users/login" -Method POST -Body $loginData -Headers $headers
    
    if ($loginResponse.access_token) {
        Write-Host "登录成功" -ForegroundColor Green
        $token = $loginResponse.access_token
        
        # 步骤2: 获取客户端列表
        Write-Host "步骤2: 获取客户端列表" -ForegroundColor Yellow
        
        $authHeaders = @{
            "Authorization2" = "Bearer $token"
            "Content-Type" = "application/json"
        }
        
        $clientsResponse = Invoke-RestMethod -Uri "$serverUrl/api/client/clientslist" -Method GET -Headers $authHeaders
        
        if ($clientsResponse.data -and $clientsResponse.data.Count -gt 0) {
            $onlineClient = $null
            foreach ($client in $clientsResponse.data) {
                if ($client.Online -eq "1") {
                    $onlineClient = $client
                    break
                }
            }
            
            if ($onlineClient) {
                Write-Host "找到在线客户端: $($onlineClient.Uid.Substring(0,8))..." -ForegroundColor Green
                
                # 步骤3: 创建测试隧道
                Write-Host "步骤3: 创建测试隧道" -ForegroundColor Yellow
                
                $tunnelData = @{
                    client_uid = $onlineClient.Uid
                    protocol = "tcp"
                    server_port = 9996
                    note = "诊断测试隧道"
                    username = ""
                    password = ""
                } | ConvertTo-Json
                
                try {
                    $createResponse = Invoke-RestMethod -Uri "$serverUrl/api/tunnel-proxy/" -Method POST -Body $tunnelData -Headers $authHeaders
                    
                    if ($createResponse.data -and $createResponse.data.id) {
                        $tunnelId = $createResponse.data.id
                        Write-Host "隧道创建成功: ID=$tunnelId" -ForegroundColor Green
                        
                        # 步骤4: 启动隧道（关键步骤）
                        Write-Host "步骤4: 启动隧道 (关键测试)" -ForegroundColor Yellow
                        
                        try {
                            $startResponse = Invoke-RestMethod -Uri "$serverUrl/api/tunnel-proxy/$tunnelId/start" -Method POST -Headers $authHeaders
                            Write-Host "SUCCESS: 隧道启动成功！" -ForegroundColor Green
                            Write-Host "响应: $($startResponse.message)" -ForegroundColor White
                            
                            # 清理
                            Invoke-RestMethod -Uri "$serverUrl/api/tunnel-proxy/$tunnelId/stop" -Method POST -Headers $authHeaders | Out-Null
                            Invoke-RestMethod -Uri "$serverUrl/api/tunnel-proxy/$tunnelId" -Method DELETE -Headers $authHeaders | Out-Null
                            
                        } catch {
                            Write-Host "FAIL: 隧道启动失败" -ForegroundColor Red
                            Write-Host "错误: $($_.Exception.Message)" -ForegroundColor Red
                            if ($_.Exception.Response) {
                                $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
                                $responseBody = $reader.ReadToEnd()
                                Write-Host "响应详情: $responseBody" -ForegroundColor Red
                            }
                            
                            # 清理
                            try {
                                Invoke-RestMethod -Uri "$serverUrl/api/tunnel-proxy/$tunnelId" -Method DELETE -Headers $authHeaders | Out-Null
                            } catch {}
                        }
                        
                    } else {
                        Write-Host "创建隧道响应异常" -ForegroundColor Red
                    }
                } catch {
                    Write-Host "创建隧道失败: $($_.Exception.Message)" -ForegroundColor Red
                }
                
            } else {
                Write-Host "没有找到在线客户端" -ForegroundColor Red
            }
        } else {
            Write-Host "获取客户端列表失败" -ForegroundColor Red
        }
        
    } else {
        Write-Host "登录失败: 没有返回token" -ForegroundColor Red
    }
} catch {
    Write-Host "登录失败: $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host "诊断完成" -ForegroundColor Cyan