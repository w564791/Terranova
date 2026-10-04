package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORS_ExposesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS(), ErrorHandler())
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	for _, method := range []string{"GET", "OPTIONS"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, "/x", nil))
		if got := w.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(got, "X-Request-ID") {
			t.Fatalf("%s: Access-Control-Expose-Headers = %q", method, got)
		}
		if got := w.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-Request-ID") {
			t.Fatalf("%s: Access-Control-Allow-Headers = %q", method, got)
		}
	}
}
