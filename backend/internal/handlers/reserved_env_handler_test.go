package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestListReservedEnvPrefixes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/system/reserved-env-prefixes", NewReservedEnvHandler().ListReservedEnvPrefixes)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/system/reserved-env-prefixes", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("code %d", w.Code)
	}
	var body struct {
		Prefixes []string `json:"prefixes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Prefixes) == 0 {
		t.Fatal("empty prefixes")
	}
	found := false
	for _, p := range body.Prefixes {
		if p == "TF_CLI_ARGS" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing TF_CLI_ARGS: %v", body.Prefixes)
	}
}
