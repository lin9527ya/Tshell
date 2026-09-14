package errors

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// maxWrapperBodySize 响应包装缓冲上限。
// JSON API 响应（客户端列表、shell 内容等）通常远小于该值；
// 超过上限（大二进制/异常大响应）直接透传，不做全量缓冲。
const maxWrapperBodySize = 8 * 1024 * 1024 // 8MB

// wrapperBodyWriter 捕获 JSON 响应体（限长）与状态码，供 ResponseWrapper 标准化后统一写出。
// 只缓冲、不落底层：包装器完成重组后再写回 c.Writer。
type wrapperBodyWriter struct {
	gin.ResponseWriter
	buf       *bytes.Buffer
	exceed    bool
	status    int
	wroteHead bool
}

func (w *wrapperBodyWriter) WriteHeader(code int) {
	w.status = code
	w.wroteHead = true
}

// WriteHeaderNow 兼容 gin ResponseWriter 接口（WriteHeader 已拦截记录）
func (w *wrapperBodyWriter) WriteHeaderNow() {}

// Written 报告响应是否已提交（基于本地拦截的状态）
func (w *wrapperBodyWriter) Written() bool { return w.wroteHead }

func (w *wrapperBodyWriter) Write(b []byte) (int, error) {
	if w.exceed {
		// 已进入透传模式：直接写底层，不再缓冲
		return w.ResponseWriter.Write(b)
	}
	if w.buf.Len()+len(b) <= maxWrapperBodySize {
		w.buf.Write(b)
		return len(b), nil
	}
	// 超限：先把已缓冲内容回写底层，再进入透传模式（避免大响应被静默丢弃）
	w.exceed = true
	if w.status != 0 && !w.ResponseWriter.Written() {
		w.ResponseWriter.WriteHeader(w.status)
	}
	if w.buf.Len() > 0 {
		w.ResponseWriter.Write(w.buf.Bytes())
		w.buf.Reset()
	}
	return w.ResponseWriter.Write(b)
}

// ResponseWrapper 统一响应格式中间件。
// 历史接口大多直接 c.JSON(gin.H{"status": 200, "data": ...}) 或返回裸数据，
// 与统一契约 {success, code, message, data, timestamp} 并存。本中间件对 JSON 响应标准化：
//   - 已含 success 字段（统一契约/错误响应）→ 透传
//   - 含 status 字段（存量格式）→ 提取 code/data/message 重组，删除顶层 status
//   - 其他纯数据（数组/字符串/对象）→ 包进 data
//
// 非 application/json（静态资源/下载 blob/流式/WebSocket 升级）与大响应直接透传。
func ResponseWrapper() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		bodyWriter := &wrapperBodyWriter{
			ResponseWriter: c.Writer,
			buf:            &bytes.Buffer{},
		}
		c.Writer = bodyWriter

		c.Next()

		// 【关键】恢复原始 writer：ErrorHandlerMiddleware 等外层中间件会在 c.Next()
		// 之后写响应（如 AbortWithError 触发的统一错误响应），若不恢复，
		// 它们会写到已返回的 wrapper 上导致响应被丢弃（空 body）。
		c.Writer = bodyWriter.ResponseWriter

		if bodyWriter.exceed {
			// 超限时已进入透传模式（缓冲内容已在 Write 中回写底层），无需额外处理
			return
		}

		ct := c.Writer.Header().Get("Content-Type")
		raw := bodyWriter.buf.Bytes()
		if !strings.Contains(ct, "application/json") || len(bytes.TrimSpace(raw)) == 0 {
			// 非 JSON/空响应透传
			if bodyWriter.status != 0 && !bodyWriter.ResponseWriter.Written() {
				bodyWriter.ResponseWriter.WriteHeader(bodyWriter.status)
			}
			if len(raw) > 0 {
				bodyWriter.ResponseWriter.Write(raw)
			}
			return
		}

		// 已写响应（如 handler 中途 flush）则透传
		if bodyWriter.ResponseWriter.Written() {
			return
		}

		var data interface{}
		code := http.StatusOK
		message := ""

		var obj map[string]interface{}
		if err := json.Unmarshal(raw, &obj); err == nil {
			// 已是统一格式（含 success 字段）→ 透传
			if _, has := obj["success"]; has {
				if bodyWriter.status != 0 {
					bodyWriter.ResponseWriter.WriteHeader(bodyWriter.status)
				}
				bodyWriter.ResponseWriter.Write(raw)
				return
			}
			if s, ok := obj["status"].(float64); ok {
				code = int(s)
			}
			if m, ok := obj["message"].(string); ok && m != "" {
				message = m
			}
			if d, ok := obj["data"]; ok {
				data = d
			} else {
				// 存量 {status, message, ...} 形式的额外字段并入 data
				extra := make(map[string]interface{})
				for k, v := range obj {
					if k == "status" || k == "message" {
						continue
					}
					extra[k] = v
				}
				if len(extra) > 0 {
					data = extra
				}
			}
		} else {
			// 数组/字符串等裸数据
			_ = json.Unmarshal(raw, &data)
		}

		if message == "" {
			if code >= 400 {
				// 错误响应：data 为字符串时直接作为 message（如 {error: "..."} 场景）
				if s, ok := data.(string); ok && s != "" {
					message = s
					data = nil
				} else {
					message = "请求失败"
				}
			} else {
				message = "ok"
			}
		}

		resp := ResponseFormat{
			Success:   code >= 200 && code < 300,
			Code:      ErrorCode(code),
			Message:   message,
			Data:      data,
			Timestamp: time.Now().Unix(),
		}
		out, err := json.Marshal(resp)
		if err != nil {
			return
		}

		status := bodyWriter.status
		if status == 0 {
			status = http.StatusOK
		}
		bodyWriter.ResponseWriter.Header().Set("Content-Length", itoa(len(out)))
		bodyWriter.ResponseWriter.WriteHeader(status)
		bodyWriter.ResponseWriter.Write(out)
	}
}

// itoa 轻量整数转字符串
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
