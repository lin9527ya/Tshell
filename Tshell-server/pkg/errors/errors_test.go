package errors

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAppErrorAndResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("AppError Error Formatting", func(t *testing.T) {
		err := NewBadRequestError("请求参数错误").WithDetail("id 不能为空")
		if err.Error() != "[1000] 请求参数错误: id 不能为空" {
			t.Fatalf("错误输出格式不符合预期: %s", err.Error())
		}
	})

	t.Run("RespondWithSuccess Helper", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Set("request_id", "req-test-123")
		c.Set("timestamp", int64(1600000000))

		testData := map[string]string{"status": "ok"}
		RespondWithSuccess(c, testData)

		if w.Code != http.StatusOK {
			t.Fatalf("预期状态码 200，实际: %d", w.Code)
		}

		var resp ResponseFormat
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("解析响应 JSON 失败: %v", err)
		}

		if !resp.Success || resp.Code != ErrCodeSuccess || resp.RequestID != "req-test-123" {
			t.Fatalf("响应数据结构校验失败: %+v", resp)
		}
	})
}
