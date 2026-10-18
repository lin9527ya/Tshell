package api

import (
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/cred"
	"BackendTemplate/pkg/database"

	"github.com/gin-gonic/gin"
)

// 凭据获取的服务端链路：下发 → 等待回传 → 制品校验 → 任务/制品/审计归属。
// 这里守的是"归属正确 + 失败显式"：制品必须对上具体请求与任务，
// 半截文件/空文件绝不能登记成"已获取"。

func setupCredTest(t *testing.T) *cred.Registry {
	t.Helper()
	setupTaskTest(t)
	old := cred.Global
	cred.Global = cred.NewRegistry()
	t.Cleanup(func() { cred.Global = old })

	for _, c := range []database.Clients{
		{Uid: "uid-win", Computer: "DC01", Os: "windows", Online: "1"},
		{Uid: "uid-lin", Computer: "web01", Os: "linux", Online: "1"},
	} {
		if _, err := database.Engine.Insert(&c); err != nil {
			t.Fatalf("插入客户端失败: %v", err)
		}
	}
	return cred.Global
}

func credRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if u := c.GetHeader("X-Test-User"); u != "" {
			c.Set("username", u)
		}
		c.Next()
	})
	r.POST("/cred/dump", DumpCredential)
	r.GET("/cred/artifacts", ListCredArtifacts)
	return r
}

func decodeInto(t *testing.T, w *httptest.ResponseRecorder, out interface{}) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
		t.Fatalf("解析响应失败: %v（%s）", err, w.Body.String())
	}
}

// minidumpBytes 构造一个可通过校验的最小 minidump（SystemInfo + Memory64List）
func minidumpBytes() []byte {
	const headerSize = 32
	const dirEntry = 12
	streams := []uint32{7, 9}
	dirRVA := uint32(headerSize)
	dataRVA := uint32(headerSize + len(streams)*dirEntry)

	header := make([]byte, headerSize)
	binary.LittleEndian.PutUint32(header[0:4], 0x504D444D)
	binary.LittleEndian.PutUint16(header[4:6], 0xA793)
	binary.LittleEndian.PutUint32(header[8:12], uint32(len(streams)))
	binary.LittleEndian.PutUint32(header[12:16], dirRVA)

	dir := make([]byte, len(streams)*dirEntry)
	data := make([]byte, 0, 64)
	for i, st := range streams {
		off := i * dirEntry
		binary.LittleEndian.PutUint32(dir[off:off+4], st)
		binary.LittleEndian.PutUint32(dir[off+4:off+8], 16)
		binary.LittleEndian.PutUint32(dir[off+8:off+12], dataRVA+uint32(len(data)))
		data = append(data, make([]byte, 16)...)
	}
	return append(append(header, dir...), data...)
}

func TestDumpCredentialValidation(t *testing.T) {
	setupCredTest(t)
	r := credRouter()

	cases := []struct {
		name string
		body string
		want int
	}{
		{"缺少 uid", `{}`, http.StatusBadRequest},
		{"会话不存在", `{"uid":"nope"}`, http.StatusNotFound},
		{"非 Windows 会话", `{"uid":"uid-lin"}`, http.StatusBadRequest},
		{"类型不支持", `{"uid":"uid-win","kind":"sam"}`, http.StatusBadRequest},
		{"技术不支持", `{"uid":"uid-win","tech":"magic"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, r, http.MethodPost, "/cred/dump", "alice", tc.body)
			if w.Code != tc.want {
				t.Fatalf("状态码 = %d，期望 %d（%s）", w.Code, tc.want, w.Body.String())
			}
		})
	}
	// 被拒绝的请求不应留下任何待回传记录
	if n := len(cred.Global.Pending("")); n != 0 {
		t.Fatalf("被拒请求不应登记待回传: %d", n)
	}
}

func TestDumpCredentialHappyPath(t *testing.T) {
	setupCredTest(t)
	r := credRouter()

	w := doJSON(t, r, http.MethodPost, "/cred/dump", "alice", `{"uid":"uid-win","kind":"lsass","tech":"dbgcore"}`)
	// 无监听器/连接时下发会失败（503），此时必须回滚登记与任务，不能留下"等待中"的幽灵
	if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable {
		t.Fatalf("状态码 = %d（%s）", w.Code, w.Body.String())
	}
	if w.Code == http.StatusServiceUnavailable {
		if n := len(cred.Global.Pending("")); n != 0 {
			t.Fatalf("下发失败后不应残留待回传记录: %d", n)
		}
		var tasks []database.Task
		database.Engine.Find(&tasks)
		if len(tasks) != 1 || tasks[0].Status != taskStatusFailed {
			t.Fatalf("下发失败应留下 failed 任务: %+v", tasks)
		}
		return
	}

	// 成功路径：任务 + 待回传记录 + 制品名
	var resp struct {
		TaskID int64 `json:"task_id"`
		Data   struct {
			FileName string `json:"file_name"`
			Kind     string `json:"kind"`
		} `json:"data"`
	}
	decodeInto(t, w, &resp)
	if resp.TaskID <= 0 || resp.Data.FileName == "" || resp.Data.Kind != cred.KindLsass {
		t.Fatalf("响应不符: %+v", resp)
	}
	pend := cred.Global.Pending("uid-win")
	if len(pend) != 1 || pend[0].TaskID != resp.TaskID || pend[0].Operator != "alice" {
		t.Fatalf("待回传记录不符: %+v", pend)
	}
	// 同一秒内重复请求同名制品 → 409（否则回传的制品会归属到错的任务）
	w2 := doJSON(t, r, http.MethodPost, "/cred/dump", "alice", `{"uid":"uid-win","kind":"lsass"}`)
	if w2.Code != http.StatusConflict {
		t.Fatalf("同名请求状态码 = %d，期望 409", w2.Code)
	}
}

// issueCredRequest 直接登记一次凭据请求并建对应任务。
// 端点本身的下发路径依赖真实传输通道（单测里没有），而回传链路才是要守的重点：
// 制品归属、校验、任务收敛都在 OnDownloadEnd 一侧，这里按同样的方式登记即可覆盖。
func issueCredRequest(t *testing.T, uid, operator string) (int64, string) {
	t.Helper()
	_, rows := createTaskRows([]string{uid}, operator, "cred", "lsass dbgcore")
	var taskID int64
	if len(rows) > 0 {
		taskID = rows[0].Id
	}
	fileName := cred.NewFileName(cred.KindLsass, "DC01", time.Now())
	if err := cred.Global.Issue(cred.ArtifactRequest{
		UID: uid, Kind: cred.KindLsass, FileName: fileName, Tech: "dbgcore",
		TaskID: taskID, Operator: operator,
	}, credArtifactTTL); err != nil {
		t.Fatalf("登记请求失败: %v", err)
	}
	return taskID, fileName
}

func TestCredArtifactIngest(t *testing.T) {
	setupCredTest(t)
	taskID, fileName := issueCredRequest(t, "uid-win", "alice")

	// 模拟客户端经下载通道回传（落在 Downloads 目录）
	dir := t.TempDir()
	dumpPath := filepath.Join(dir, fileName)
	if err := os.WriteFile(dumpPath, minidumpBytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	OnDownloadEnd("uid-win", dumpPath, command.DownloadStatusCompleted)

	var task database.Task
	if has, _ := database.Engine.ID(taskID).Get(&task); !has || task.Status != taskStatusCompleted {
		t.Fatalf("制品回传后任务未置 completed: %+v", task)
	}
	if task.Result == "" {
		t.Fatal("完成态应带上制品说明")
	}

	var loot []database.Loot
	database.Engine.Where("uid = ?", "uid-win").Find(&loot)
	if len(loot) != 1 {
		t.Fatalf("应登记 1 条制品，实际 %d 条", len(loot))
	}
	if loot[0].Type != "dump" || loot[0].Source != cred.KindLsass || loot[0].Secret != dumpPath {
		t.Fatalf("制品字段不符: %+v", loot[0])
	}
	if loot[0].Note == "" {
		t.Fatal("制品备注应说明解析方式")
	}
	// 制品登记后不再显示"等待回传"
	if n := len(cred.Global.Pending("uid-win")); n != 0 {
		t.Fatalf("制品已回传，待回传记录应清空: %d", n)
	}
}

func TestCredArtifactRejectsInvalidDump(t *testing.T) {
	setupCredTest(t)
	taskID, fileName := issueCredRequest(t, "uid-win", "alice")

	// 客户端"成功"回传了一个空文件（权限不足时很常见）——必须显式失败，不能登记成制品
	dir := t.TempDir()
	badPath := filepath.Join(dir, fileName)
	if err := os.WriteFile(badPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	OnDownloadEnd("uid-win", badPath, command.DownloadStatusCompleted)

	var task database.Task
	database.Engine.ID(taskID).Get(&task)
	if task.Status != taskStatusFailed {
		t.Fatalf("无效制品应让任务失败，实际 %s", task.Status)
	}
	if task.Error == "" {
		t.Fatal("失败原因不能为空")
	}
	var loot []database.Loot
	database.Engine.Find(&loot)
	if len(loot) != 0 {
		t.Fatalf("无效制品不应登记为战利品: %+v", loot)
	}
	// 半截转储也要能识别（转储写一半时被中断的常见形态）
	full := minidumpBytes()
	truncPath := filepath.Join(t.TempDir(), fileName)
	if err := os.WriteFile(truncPath, full[:len(full)-8], 0o600); err != nil {
		t.Fatal(err)
	}
	taskID2, name2 := issueCredRequest(t, "uid-win", "alice")
	OnDownloadEnd("uid-win", filepath.Join(filepath.Dir(truncPath), name2), command.DownloadStatusCompleted)
	var task2 database.Task
	database.Engine.ID(taskID2).Get(&task2)
	if task2.Status != taskStatusFailed {
		t.Fatalf("截断制品应让任务失败，实际 %s", task2.Status)
	}
}

func TestCredArtifactAbortMarksFailed(t *testing.T) {
	setupCredTest(t)
	taskID, fileName := issueCredRequest(t, "uid-win", "alice")

	// 客户端断线导致制品回传中断
	OnDownloadEnd("uid-win", filepath.Join(t.TempDir(), fileName), command.DownloadStatusFailed)

	var task database.Task
	database.Engine.ID(taskID).Get(&task)
	if task.Status != taskStatusFailed {
		t.Fatalf("回传中断应让任务失败，实际 %s", task.Status)
	}
	if n := len(cred.Global.Pending("uid-win")); n != 0 {
		t.Fatalf("中断后不应残留待回传记录: %d", n)
	}
}

// TestUnrelatedDownloadIsNotCredArtifact 普通文件下载不能被误认成凭据制品
func TestUnrelatedDownloadIsNotCredArtifact(t *testing.T) {
	setupCredTest(t)
	taskID, _ := issueCredRequest(t, "uid-win", "alice")

	// 另一个文件回传完成：既不应动凭据任务，也不应登记制品
	OnDownloadEnd("uid-win", filepath.Join(t.TempDir(), "report.docx"), command.DownloadStatusCompleted)

	var loot []database.Loot
	database.Engine.Find(&loot)
	if len(loot) != 0 {
		t.Fatalf("普通下载不应登记为凭据制品: %+v", loot)
	}
	var task database.Task
	database.Engine.ID(taskID).Get(&task)
	if task.Status != taskStatusDispatched {
		t.Fatalf("无关回传不应改变凭据任务状态: %s", task.Status)
	}
	if n := len(cred.Global.Pending("uid-win")); n != 1 {
		t.Fatalf("无关回传不应消费待回传记录: %d", n)
	}
}

func TestListCredArtifacts(t *testing.T) {
	setupCredTest(t)
	r := credRouter()

	w := doJSON(t, r, http.MethodGet, "/cred/artifacts", "alice", "")
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", w.Code)
	}
	var resp struct {
		Data struct {
			Pending   []cred.ArtifactRequest `json:"pending"`
			Artifacts []database.Loot        `json:"artifacts"`
		} `json:"data"`
	}
	decodeInto(t, w, &resp)
	if len(resp.Data.Pending) != 0 || len(resp.Data.Artifacts) != 0 {
		t.Fatalf("初始状态应为空: %+v", resp.Data)
	}

	// 手工登记一条待回传 + 一条已落地制品，接口应能分别返回
	if err := cred.Global.Issue(cred.ArtifactRequest{
		UID: "uid-win", Kind: cred.KindLsass, FileName: "lsass-x.dmp", Operator: "bob",
	}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Engine.Insert(&database.Loot{
		Uid: "uid-win", Type: "dump", Source: cred.KindLsass, Secret: "/srv/Downloads/1/x.dmp",
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
	}); err != nil {
		t.Fatal(err)
	}

	w = doJSON(t, r, http.MethodGet, "/cred/artifacts?uid=uid-win", "alice", "")
	decodeInto(t, w, &resp)
	if len(resp.Data.Pending) != 1 || resp.Data.Pending[0].FileName != "lsass-x.dmp" {
		t.Fatalf("待回传列表不符: %+v", resp.Data.Pending)
	}
	if len(resp.Data.Artifacts) != 1 || resp.Data.Artifacts[0].Type != "dump" {
		t.Fatalf("制品列表不符: %+v", resp.Data.Artifacts)
	}
}
