package api

import (
	"archive/zip"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"BackendTemplate/pkg/command"
	credpkg "BackendTemplate/pkg/cred"
	"BackendTemplate/pkg/database"
)

// 第二批凭据模块（DPAPI / 浏览器）的服务端链路：
// 类型与技术组合校验、按制品形态分派校验、明文口令入库与去重、制品列表可见性。
// 与 LSASS 一样守两条底线：失败必须显式、归属必须准确。

func TestDumpCredentialKindTechMatrix(t *testing.T) {
	setupCredTest(t)
	r := credRouter()

	cases := []struct {
		name string
		body string
		want []int // 允许的状态码（无传输通道时下发会 503）
	}{
		{"browser+comsvcs 跨类型误配", `{"uid":"uid-win","kind":"browser","tech":"comsvcs"}`, []int{http.StatusBadRequest}},
		{"dpapi+dbgcore 跨类型误配", `{"uid":"uid-win","kind":"dpapi","tech":"dbgcore"}`, []int{http.StatusBadRequest}},
		{"lsass+chrome 跨类型误配", `{"uid":"uid-win","kind":"lsass","tech":"chrome"}`, []int{http.StatusBadRequest}},
		{"browser+chrome 合法", `{"uid":"uid-win","kind":"browser","tech":"chrome"}`, []int{http.StatusOK, http.StatusServiceUnavailable}},
		{"browser 省略技术取默认 all", `{"uid":"uid-win","kind":"browser"}`, []int{http.StatusOK, http.StatusServiceUnavailable}},
		{"dpapi+keys 合法", `{"uid":"uid-win","kind":"dpapi","tech":"keys"}`, []int{http.StatusOK, http.StatusServiceUnavailable}},
		{"非 Windows 会话", `{"uid":"uid-lin","kind":"browser"}`, []int{http.StatusBadRequest}},
		{"类型仍不支持", `{"uid":"uid-win","kind":"sam"}`, []int{http.StatusBadRequest}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, r, http.MethodPost, "/cred/dump", "alice", tc.body)
			ok := false
			for _, want := range tc.want {
				if w.Code == want {
					ok = true
				}
			}
			if !ok {
				t.Fatalf("状态码 = %d，期望 %v（%s）", w.Code, tc.want, w.Body.String())
			}
		})
	}
	// 合法请求下发失败时会回滚登记：不允许留下"等待中"的幽灵记录
	if n := len(credpkg.Global.Pending("")); n != 0 {
		t.Fatalf("下发失败的请求不应残留待回传: %d", n)
	}
}

// issueKindRequest 登记一次指定类型的凭据请求（制品名按类型生成）
func issueKindRequest(t *testing.T, uid, kind, tech string) (int64, string) {
	t.Helper()
	_, rows := createTaskRows([]string{uid}, "alice", "cred", kind+" "+tech)
	var taskID int64
	if len(rows) > 0 {
		taskID = rows[0].Id
	}
	fileName := credpkg.NewFileName(kind, "WS01", time.Now())
	if err := credpkg.Global.Issue(credpkg.ArtifactRequest{
		UID: uid, Kind: kind, FileName: fileName, Tech: tech, TaskID: taskID,
		Operator: "alice", Format: credpkg.FormatByKind(kind),
	}, credArtifactTTL); err != nil {
		t.Fatalf("登记请求失败: %v", err)
	}
	return taskID, fileName
}

func sampleBrowserJSON(t *testing.T) []byte {
	t.Helper()
	art := credpkg.BrowserArtifact{
		Host: "WS01", User: `CORP\alice`, CollectedAt: "2026-10-18 13:14:15",
		Errors: []string{"edge: 未安装"},
		Browsers: []credpkg.BrowserProfile{
			{
				Name: "chrome", Profile: "Default", Tech: "v20", MasterKey: "AAECAwQ=",
				Credentials: []credpkg.BrowserCredential{
					{URL: "https://portal.corp.local/login", User: "alice", Pass: "S3cr3t!"},
					{URL: "https://portal.corp.local/login", User: "alice", Pass: "S3cr3t!"}, // 重复 → 去重
					{URL: "", User: "nobody", Pass: "dropped"},                               // 缺 URL → 丢弃
				},
				Cookies: []credpkg.BrowserCookie{
					{Host: ".corp.local", Name: "SESSIONID", Value: "abc123", Path: "/", HTTPOnly: true},
				},
			},
		},
	}
	raw, err := json.Marshal(art)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestBrowserArtifactIngest(t *testing.T) {
	setupCredTest(t)
	taskID, fileName := issueKindRequest(t, "uid-win", credpkg.KindBrowser, "all")

	path := filepath.Join(t.TempDir(), fileName)
	if err := os.WriteFile(path, sampleBrowserJSON(t), 0o600); err != nil {
		t.Fatal(err)
	}
	OnDownloadEnd("uid-win", path, command.DownloadStatusCompleted)

	var task database.Task
	database.Engine.ID(taskID).Get(&task)
	if task.Status != taskStatusCompleted {
		t.Fatalf("制品回传后任务应 completed: %+v", task)
	}

	var loot []database.Loot
	database.Engine.Where("uid = ?", "uid-win").Asc("id").Find(&loot)
	if len(loot) != 3 {
		t.Fatalf("应登记 3 条（1 条口令去重后 + 1 条 Cookie + 1 条制品本体），实际 %d: %+v", len(loot), loot)
	}

	cred := loot[0]
	if cred.Type != "pwd" || cred.User != "alice" || cred.Secret != "S3cr3t!" ||
		cred.Source != "browser:chrome" || cred.Realm != "portal.corp.local" || cred.Host != "WS01" {
		t.Fatalf("口令条目字段不符: %+v", cred)
	}
	if cred.Note == "" {
		t.Fatal("口令条目应带来源说明")
	}

	cookie := loot[1]
	if cookie.Type != "cookie" || cookie.Secret != "SESSIONID=abc123" ||
		cookie.Source != "cookie:chrome" || cookie.Realm != ".corp.local" {
		t.Fatalf("Cookie 条目字段不符: %+v", cookie)
	}

	artifact := loot[2]
	if artifact.Type != "dump" || artifact.Source != credpkg.KindBrowser || artifact.Secret != path {
		t.Fatalf("制品本体条目不符: %+v", artifact)
	}

	// 重复采集（同一份制品再回传一次）不应把库刷满
	taskID2, fileName2 := issueKindRequest(t, "uid-win", credpkg.KindBrowser, "all")
	path2 := filepath.Join(t.TempDir(), fileName2)
	if err := os.WriteFile(path2, sampleBrowserJSON(t), 0o600); err != nil {
		t.Fatal(err)
	}
	OnDownloadEnd("uid-win", path2, command.DownloadStatusCompleted)

	var after []database.Loot
	database.Engine.Where("uid = ? AND type = ?", "uid-win", "pwd").Find(&after)
	if len(after) != 1 {
		t.Fatalf("重复采集不应重复入库口令: %d 条", len(after))
	}
	var task2 database.Task
	database.Engine.ID(taskID2).Get(&task2)
	if task2.Status != taskStatusCompleted {
		t.Fatalf("第二次采集任务也应完成（制品本体是新的）: %+v", task2)
	}
}

func TestBrowserArtifactRejectsMalformed(t *testing.T) {
	// 半截 JSON / 无可用条目 → 任务失败且不登记任何制品/凭据
	bodies := map[string][]byte{
		"半截 JSON": []byte(`{"host":"WS01","browsers":[{"name":"chrome","credentials":[{"url":"https://x/","user":"a","pass":"b"}`),
		"无可用条目":   []byte(`{"host":"WS01","browsers":[{"name":"chrome","credentials":[],"cookies":[]}]}`),
		"不是 JSON": []byte("PK\x03\x04 not json at all"),
		"空文件":     nil,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			setupCredTest(t)
			taskID, fileName := issueKindRequest(t, "uid-win", credpkg.KindBrowser, "all")
			path := filepath.Join(t.TempDir(), fileName)
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			OnDownloadEnd("uid-win", path, command.DownloadStatusCompleted)

			var task database.Task
			database.Engine.ID(taskID).Get(&task)
			if task.Status != taskStatusFailed || task.Error == "" {
				t.Fatalf("畸形制品必须显式失败: %+v", task)
			}
			var loot []database.Loot
			database.Engine.Find(&loot)
			if len(loot) != 0 {
				t.Fatalf("畸形制品不应入库: %+v", loot)
			}
		})
	}
}

func TestDpapiArtifactIngestAndRejects(t *testing.T) {
	setupCredTest(t)

	// 合法：含 manifest.json 的 ZIP
	taskID, fileName := issueKindRequest(t, "uid-win", credpkg.KindDPAPI, "keys")
	good := filepath.Join(t.TempDir(), fileName)
	writeZipArtifact(t, good, map[string]string{
		"manifest.json":    `{"host":"WS01","user":"alice","keys":["guid-1"]}`,
		"keys/guid-1.json": `{"guid":"guid-1","masterkey":"0a1b2c"}`,
	})
	OnDownloadEnd("uid-win", good, command.DownloadStatusCompleted)

	var task database.Task
	database.Engine.ID(taskID).Get(&task)
	if task.Status != taskStatusCompleted {
		t.Fatalf("合法 DPAPI 制品应完成: %+v", task)
	}
	var loot []database.Loot
	database.Engine.Where("uid = ?", "uid-win").Find(&loot)
	if len(loot) != 1 || loot[0].Type != "dump" || loot[0].Source != credpkg.KindDPAPI {
		t.Fatalf("DPAPI 制品登记不符: %+v", loot)
	}
	if loot[0].Note == "" {
		t.Fatal("DPAPI 制品备注应说明用途")
	}

	// 非法：截断的 ZIP（只剩头部）
	taskID2, name2 := issueKindRequest(t, "uid-win", credpkg.KindDPAPI, "keys")
	raw, err := os.ReadFile(good)
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(t.TempDir(), name2)
	if err := os.WriteFile(bad, raw[:len(raw)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	OnDownloadEnd("uid-win", bad, command.DownloadStatusCompleted)

	var task2 database.Task
	database.Engine.ID(taskID2).Get(&task2)
	if task2.Status != taskStatusFailed {
		t.Fatalf("截断 ZIP 必须失败: %+v", task2)
	}
	count, _ := database.Engine.Where("uid = ? AND source = ?", "uid-win", credpkg.KindDPAPI).Count(&database.Loot{})
	if count != 1 {
		t.Fatalf("截断制品不应新增登记: %d", count)
	}
}

func TestListCredArtifactsCoversAllKinds(t *testing.T) {
	setupCredTest(t)
	r := credRouter()

	// 三种类型的已落地制品都应出现在弹窗数据里（此前只过滤 lsass，新模块会"看不见"）
	for _, kind := range credpkg.Kinds {
		if _, err := database.Engine.Insert(&database.Loot{
			Uid: "uid-win", Type: "dump", Source: kind,
			Secret: "/srv/Downloads/1/" + kind, CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
		}); err != nil {
			t.Fatal(err)
		}
	}
	// 无关来源（普通下载/wifi）不应混入
	if _, err := database.Engine.Insert(&database.Loot{
		Uid: "uid-win", Type: "note", Source: "manual",
		Secret: "not-an-artifact", CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
	}); err != nil {
		t.Fatal(err)
	}

	w := doJSON(t, r, http.MethodGet, "/cred/artifacts?uid=uid-win", "alice", "")
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", w.Code)
	}
	var resp struct {
		Data struct {
			Artifacts []database.Loot `json:"artifacts"`
		} `json:"data"`
	}
	decodeInto(t, w, &resp)
	if len(resp.Data.Artifacts) != len(credpkg.Kinds) {
		t.Fatalf("制品列表应含 %d 种来源，实际 %d: %+v",
			len(credpkg.Kinds), len(resp.Data.Artifacts), resp.Data.Artifacts)
	}
	seen := map[string]bool{}
	for _, a := range resp.Data.Artifacts {
		seen[a.Source] = true
	}
	for _, kind := range credpkg.Kinds {
		if !seen[kind] {
			t.Fatalf("来源 %s 未出现在列表中: %+v", kind, seen)
		}
	}
}

// writeZipArtifact 造一个含 manifest.json 的 ZIP 制品
func writeZipArtifact(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
