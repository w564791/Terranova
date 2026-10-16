package services

import (
	"strings"
	"testing"

	"iac-platform/internal/models"
)

type stripEnvAccessor struct {
	DataAccessor
	vars []models.WorkspaceVariable
}

func (a stripEnvAccessor) GetWorkspaceVariables(string, models.VariableType) ([]models.WorkspaceVariable, error) {
	return a.vars, nil
}

func TestBuildEnvironmentVariables_StripsReserved(t *testing.T) {
	s := &TerraformExecutor{
		dataAccessor: stripEnvAccessor{vars: []models.WorkspaceVariable{
			{Key: "AWS_REGION", Value: "us-east-1", VariableType: models.VariableTypeEnvironment},
			{Key: "TF_CLI_ARGS_init", Value: "-upgrade", VariableType: models.VariableTypeEnvironment},
			{Key: "HTTP_PROXY", Value: "http://evil", VariableType: models.VariableTypeEnvironment},
			{Key: "TF_IN_AUTOMATION", Value: "false", VariableType: models.VariableTypeEnvironment},
		}},
	}
	t.Setenv("IAC_EXEC_HTTP_PROXY", "http://platform-proxy:8080")
	t.Setenv("IAC_EXEC_HTTPS_PROXY", "")
	t.Setenv("IAC_EXEC_NO_PROXY", "")
	env := s.buildEnvironmentVariables(&models.Workspace{WorkspaceID: "ws-1"})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "TF_CLI_ARGS_init=") {
		t.Fatal("TF_CLI_ARGS_init must be stripped")
	}
	if strings.Contains(joined, "HTTP_PROXY=http://evil") {
		t.Fatal("user HTTP_PROXY must be stripped")
	}
	if !strings.Contains(joined, "HTTP_PROXY=http://platform-proxy:8080") {
		t.Fatal("platform proxy must be injected")
	}
	if !strings.Contains(joined, "AWS_REGION=us-east-1") {
		t.Fatal("non-reserved must remain")
	}
	if !strings.Contains(joined, "TF_IN_AUTOMATION=true") {
		t.Fatal("platform TF_IN_AUTOMATION must win")
	}
	if args := s.getTFCLIArgs("ws-1"); len(args) != 0 {
		t.Fatalf("getTFCLIArgs must be empty, got %v", args)
	}
}

func TestBuildEnvironmentVariables_PlatformCAAfterReservedStrip(t *testing.T) {
	// Legacy DB row still holds TF_HTTP_CLIENT_CA_CERTIFICATE_PEM (reserved).
	// After strip it must not block injecting platform _TERRANOVA_CA_CERT.
	const platformCA = "-----BEGIN CERTIFICATE-----\nPLATFORM\n-----END CERTIFICATE-----\n"
	const legacyUserCA = "-----BEGIN CERTIFICATE-----\nUSER\n-----END CERTIFICATE-----\n"
	s := &TerraformExecutor{
		stateBackendURL: "https://state.example/api/v1/state",
		stateToken:      "tok",
		dataAccessor: stripEnvAccessor{vars: []models.WorkspaceVariable{
			{Key: "TF_HTTP_CLIENT_CA_CERTIFICATE_PEM", Value: legacyUserCA, VariableType: models.VariableTypeEnvironment},
			{Key: "AWS_REGION", Value: "us-east-1", VariableType: models.VariableTypeEnvironment},
		}},
	}
	t.Setenv("_TERRANOVA_CA_CERT", platformCA)
	env := s.buildEnvironmentVariables(&models.Workspace{WorkspaceID: "ws-1"})
	var caVals []string
	for _, e := range env {
		key, val, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(key, "TF_HTTP_CLIENT_CA_CERTIFICATE_PEM") {
			caVals = append(caVals, val)
		}
		if strings.EqualFold(key, "TF_HTTP_CLIENT_CA_CERTIFICATE_PEM") && val == legacyUserCA {
			t.Fatal("reserved user TF_HTTP_CLIENT_CA_CERTIFICATE_PEM must be stripped")
		}
	}
	if len(caVals) != 1 {
		t.Fatalf("want exactly one TF_HTTP_CLIENT_CA_CERTIFICATE_PEM, got %v", caVals)
	}
	if caVals[0] != platformCA {
		t.Fatalf("platform CA not injected: got %q", caVals[0])
	}
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "AWS_REGION=us-east-1") {
		t.Fatal("non-reserved must remain")
	}
}

