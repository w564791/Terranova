package tlstrust

import (
	"os"
	"strings"
	"testing"
)

func TestResolveAgentProtocol(t *testing.T) {
	got, err := ResolveAgentProtocol("")
	if err != nil || got != "http" {
		t.Fatalf("default: got=%q err=%v", got, err)
	}
	got, err = ResolveAgentProtocol("HTTPS")
	if err != nil || got != "https" {
		t.Fatalf("https: got=%q err=%v", got, err)
	}
	if _, err := ResolveAgentProtocol("ftp"); err == nil {
		t.Fatal("ftp must be rejected")
	}
}

func TestWebSocketScheme(t *testing.T) {
	if WebSocketScheme("https") != "wss" || WebSocketScheme("http") != "ws" {
		t.Fatal("scheme mapping")
	}
}

func TestCheckAgentPlaintext_Production(t *testing.T) {
	clean(t)
	t.Setenv("ENV", "production")
	os.Unsetenv(EnvAgentAllowPlaintext)

	_, err := CheckAgentPlaintext("http")
	if err == nil || !strings.Contains(err.Error(), "refusing to start") {
		t.Fatalf("http without allow: err=%v", err)
	}

	t.Setenv(EnvAgentAllowPlaintext, "yes")
	_, err = CheckAgentPlaintext("http")
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("bad allow value: err=%v", err)
	}

	t.Setenv(EnvAgentAllowPlaintext, AllowPlaintextClusterInternal)
	w, err := CheckAgentPlaintext("http")
	if err != nil || len(w) == 0 || !strings.Contains(w[0], "cluster-internal") {
		t.Fatalf("cluster-internal allow: warnings=%v err=%v", w, err)
	}

	os.Unsetenv(EnvAgentAllowPlaintext)
	w, err = CheckAgentPlaintext("https")
	if err != nil || len(w) != 0 {
		t.Fatalf("https: warnings=%v err=%v", w, err)
	}
}

func TestCheckAgentPlaintext_Development(t *testing.T) {
	clean(t)
	t.Setenv("ENV", "development")
	os.Unsetenv(EnvAgentAllowPlaintext)
	w, err := CheckAgentPlaintext("http")
	if err != nil || len(w) == 0 {
		t.Fatalf("dev http: warnings=%v err=%v", w, err)
	}
}
