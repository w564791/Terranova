package services

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iac-platform/internal/models"

	corev1 "k8s.io/api/core/v1"
)

type envVarsAccessor struct {
	DataAccessor
	vars []models.WorkspaceVariable
}

func (a envVarsAccessor) GetWorkspaceVariables(string, models.VariableType) ([]models.WorkspaceVariable, error) {
	return a.vars, nil
}

func tlsTestCAFile(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func envValue(env []string, name string) (string, bool) {
	val, found := "", false
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			val, found = v, true // last wins, as for exec
		}
	}
	return val, found
}

func TestStateBackendTrustsIACCAFile(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()
	t.Setenv("_TERRANOVA_CA_CERT", "")
	t.Setenv("AGENT_CA_FILE", "")
	ca := tlsTestCAFile(t, srv)
	t.Setenv("IAC_CA_FILE", ca)

	s := &TerraformExecutor{dataAccessor: envVarsAccessor{}, stateBackendURL: "https://platform.example/state", stateToken: "tok"}
	got, ok := envValue(s.buildEnvironmentVariables(&models.Workspace{WorkspaceID: "ws-1"}), "TF_HTTP_CLIENT_CA_CERTIFICATE_PEM")
	if !ok {
		t.Fatal("IAC_CA_FILE not passed to the Terraform state backend")
	}
	want, _ := os.ReadFile(ca)
	if !strings.Contains(got, string(want)) {
		t.Fatal("state backend CA bundle lacks IAC_CA_FILE")
	}

	// Without a CA bundle nothing is injected (system roots apply).
	t.Setenv("IAC_CA_FILE", "")
	if _, ok := envValue(s.buildEnvironmentVariables(&models.Workspace{WorkspaceID: "ws-1"}), "TF_HTTP_CLIENT_CA_CERTIFICATE_PEM"); ok {
		t.Fatal("CA injected without configuration")
	}
}

func TestAgentAPIClientVerifiesAndTrustsCAFile(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()
	t.Setenv("AGENT_CA_FILE", "")

	t.Setenv("IAC_CA_FILE", "")
	c := NewAgentAPIClient(srv.URL, "tok")
	tr := c.httpClient.Transport.(*http.Transport)
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("agent API client must verify certificates")
	}
	if resp, err := c.httpClient.Get(srv.URL); err == nil {
		resp.Body.Close()
		t.Fatal("untrusted server certificate accepted")
	}

	t.Setenv("AGENT_CA_FILE", tlsTestCAFile(t, srv))
	c = NewAgentAPIClient(srv.URL, "tok")
	resp, err := c.httpClient.Get(srv.URL)
	if err != nil {
		t.Fatalf("AGENT_CA_FILE not trusted: %v", err)
	}
	resp.Body.Close()
}

func TestWithAgentMode(t *testing.T) {
	base := []corev1.EnvVar{{Name: "POOL_ID", Value: "p"}, {Name: "ENV", Value: "development"}}
	if got := withAgentMode(append([]corev1.EnvVar{}, base...), false); len(got) != 2 || got[1].Value != "development" {
		t.Fatalf("non-production must keep the template: %+v", got)
	}
	got := withAgentMode(append([]corev1.EnvVar{}, base...), true)
	n := 0
	for _, e := range got {
		if e.Name == "ENV" {
			n++
			if e.Value != "production" {
				t.Fatalf("production platform, agent ENV=%q", e.Value)
			}
		}
	}
	if n != 1 || len(got) != 2 {
		t.Fatalf("ENV must appear exactly once: %+v", got)
	}
}

func TestWithAgentPlaintextAllow(t *testing.T) {
	base := []corev1.EnvVar{{Name: "IAC_AGENT_PROTOCOL", Value: "http"}}
	if got := withAgentPlaintextAllow(append([]corev1.EnvVar{}, base...), false); len(got) != 1 {
		t.Fatalf("non-production must not inject: %+v", got)
	}
	got := withAgentPlaintextAllow(append([]corev1.EnvVar{}, base...), true)
	found := false
	for _, e := range got {
		if e.Name == "IAC_AGENT_ALLOW_PLAINTEXT" {
			found = true
			if e.Value != "cluster-internal" {
				t.Fatalf("value=%q", e.Value)
			}
		}
	}
	if !found {
		t.Fatalf("production http must inject allow flag: %+v", got)
	}
	https := []corev1.EnvVar{{Name: "IAC_AGENT_PROTOCOL", Value: "https"}}
	if got := withAgentPlaintextAllow(append([]corev1.EnvVar{}, https...), true); len(got) != 1 {
		t.Fatalf("https must not inject: %+v", got)
	}
	preset := []corev1.EnvVar{
		{Name: "IAC_AGENT_PROTOCOL", Value: "http"},
		{Name: "IAC_AGENT_ALLOW_PLAINTEXT", Value: "cluster-internal"},
	}
	if got := withAgentPlaintextAllow(append([]corev1.EnvVar{}, preset...), true); len(got) != 2 {
		t.Fatalf("must not duplicate: %+v", got)
	}
}
