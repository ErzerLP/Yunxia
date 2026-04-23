package response

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type meta struct {
	RequestID string `json:"request_id"`
	Timestamp string `json:"timestamp"`
}

type errorBody struct {
	Details any `json:"details,omitempty"`
}

// JSON 返回成功响应包。
func JSON(c *gin.Context, status int, code, message string, data any) {
	c.Set("response_code", code)
	c.JSON(status, gin.H{
		"success": true,
		"code":    code,
		"message": message,
		"data":    data,
		"meta": meta{
			RequestID: c.GetString("request_id"),
			Timestamp: time.Now().Format(time.RFC3339),
		},
	})
}

// Error 返回错误响应包。
func Error(c *gin.Context, status int, code, message string, details any) {
	c.Set("response_code", code)
	c.JSON(status, gin.H{
		"success": false,
		"code":    code,
		"message": message,
		"error": errorBody{
			Details: details,
		},
		"meta": meta{
			RequestID: c.GetString("request_id"),
			Timestamp: time.Now().Format(time.RFC3339),
		},
	})
}

// Empty 返回空 data 的成功响应。
func Empty(c *gin.Context, status int) {
	JSON(c, status, "OK", "ok", gin.H{})
}

// IsErrorStatus 判断是否为错误状态码。
func IsErrorStatus(status int) bool {
	return status >= http.StatusBadRequest
}
