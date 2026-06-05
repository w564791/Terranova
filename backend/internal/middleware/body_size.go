package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// LimitRequestBodySize 限制单次请求体大小,超过返回 413
//
// 用于 manifest 文件 PUT 等可能上传较大 body 的端点。中间件层拦截避免
// handler 内 ioutil.ReadAll 把大 body 读进内存。
func LimitRequestBodySize(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}
