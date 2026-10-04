package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func errorHandlerRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorHandler())
	r.GET("/boom", func(c *gin.Context) {
		_ = c.Error(errors.New(`pq: relation "manifest_deployments" does not exist (SQLSTATE 42P01)`))
	})
	r.GET("/ok", func(c *gin.Context) { c.String(http.StatusOK, c.GetString(RequestIDContextKey)) })
	r.GET("/handled", func(c *gin.Context) {
		_ = c.Error(errors.New("db down"))
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad"})
	})
	return r
}

func serveWithRequestID(r http.Handler, path string, id *string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	if id != nil {
		// set the map directly so values Header.Set would reject (newlines) still arrive
		req.Header[textproto.CanonicalMIMEHeaderKey(RequestIDHeader)] = []string{*id}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestErrorHandler_500BodyIsGenericWithRequestID(t *testing.T) {
	w := serveWithRequestID(errorHandlerRouter(), "/boom", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("code %d", w.Code)
	}
	body := w.Body.String()
	for _, leak := range []string{"pq:", "relation", "SQLSTATE", "manifest_deployments"} {
		if strings.Contains(body, leak) {
			t.Fatalf("500 body leaks %q: %s", leak, body)
		}
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["error"] != "internal error" || got["request_id"] == "" || got["request_id"] != w.Header().Get(RequestIDHeader) {
		t.Fatalf("body %v header %q", got, w.Header().Get(RequestIDHeader))
	}
	if _, err := uuid.Parse(got["request_id"].(string)); err != nil {
		t.Fatalf("generated id is not a UUID: %v", got["request_id"])
	}
}

func TestErrorHandler_HandlerResponseKept(t *testing.T) {
	w := serveWithRequestID(errorHandlerRouter(), "/handled", nil)
	if w.Code != http.StatusBadRequest || strings.Contains(w.Body.String(), "internal error") {
		t.Fatalf("handler response overwritten: %d %s", w.Code, w.Body.String())
	}
}

func TestErrorHandler_RequestIDValidation(t *testing.T) {
	valid := "abc-DEF-12345678"
	cases := []struct {
		name string
		in   string
		echo bool
	}{
		{"valid value is echoed", valid, true},
		{"newline is replaced", "abcdefgh\nX-Injected: 1", false},
		{"over-long is replaced", strings.Repeat("a", 65), false},
		{"64 chars is echoed", strings.Repeat("a", 64), true},
		{"too short is replaced", "abc1234", false},
		{"other characters are replaced", "abcd_efgh/ijkl", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			w := serveWithRequestID(errorHandlerRouter(), "/ok", &in)
			hdr := w.Header().Get(RequestIDHeader)
			if hdr != w.Body.String() {
				t.Fatalf("header %q != context %q", hdr, w.Body.String())
			}
			if tc.echo {
				if hdr != tc.in {
					t.Fatalf("want echo %q got %q", tc.in, hdr)
				}
				return
			}
			if hdr == tc.in || strings.ContainsAny(hdr, "\r\n") {
				t.Fatalf("invalid id must be replaced, got %q", hdr)
			}
			if _, err := uuid.Parse(hdr); err != nil {
				t.Fatalf("replacement is not a UUID: %q", hdr)
			}
		})
	}
	// the 500 body echoes a valid incoming id
	in := valid
	w := serveWithRequestID(errorHandlerRouter(), "/boom", &in)
	if !strings.Contains(w.Body.String(), `"request_id":"`+valid+`"`) {
		t.Fatalf("500 body: %s", w.Body.String())
	}
}
