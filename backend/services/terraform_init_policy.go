package services

import (
	"errors"

	"iac-platform/internal/crypto"
	"iac-platform/internal/manifestbundle"
	"iac-platform/internal/models"

	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Provider installation policy of every `terraform init` the executor runs
// (local mode and the agent, which runs the same executor):
//
//   - The dependency lock file is always honored: init never runs with
//     -upgrade. Providers that do not match the lock (version constraint or
//     package checksum) fail init; isLockedProviderError makes that final.
//   - The provider plugin cache is private to one task
//     (<workDir>/PerTaskPluginCacheDirName, recreated on the first attempt and
//     removed with the work directory). A writable cache shared between
//     workspaces / organizations (e.g. an agent pool's TF_PLUGIN_CACHE_DIR)
//     would let one tenant's run plant provider binaries that another
//     tenant's init then links, so the inherited process variable and any
//     workspace variable that changes the cache or relaxes lock checking are
//     dropped (protectedTerraformEnvKeys).
//   - The executor writes no CLI config file and never sets
//     TF_CLI_CONFIG_FILE. (A workdir .terraformrc used to be written with
//     plugin_cache_may_break_dependency_lock_file = true; Terraform never
//     read it, since it only reads $HOME/.terraformrc or TF_CLI_CONFIG_FILE,
//     and the setting would have disabled the lock checks for cached
//     providers. It is gone.)

// PerTaskPluginCacheDirName is the per-task plugin cache directory, relative
// to the task work directory.
const PerTaskPluginCacheDirName = ".terranova-plugin-cache"

// protectedTerraformEnvKeys may come neither from the executor's process
// environment nor from workspace variables:
//   - TF_PLUGIN_CACHE_DIR: always the per-task cache;
//   - TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE: would let cached
//     providers bypass the lock file checksums;
//   - TF_CLI_ARGS_init: could inject -upgrade (or other init flags).
var protectedTerraformEnvKeys = map[string]bool{
	"TF_PLUGIN_CACHE_DIR":                            true,
	"TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE": true,
	"TF_CLI_ARGS_init":                               true,
}

func isProtectedTerraformEnvKey(key string) bool {
	return protectedTerraformEnvKeys[key]
}

// sanitizeTerraformEnv drops protected KEY=VALUE entries (see
// protectedTerraformEnvKeys) from an environment list.
func sanitizeTerraformEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		if isProtectedTerraformEnvKey(key) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// preparePerTaskPluginCache returns <workDir>/PerTaskPluginCacheDirName,
// recreating it empty (mode 0700) when fresh is true so nothing placed there
// beforehand (e.g. by configuration files written into the work directory)
// is ever used as a cache entry. Retries of the same task pass fresh=false
// and reuse their own downloads.
func preparePerTaskPluginCache(workDir string, fresh bool) (string, error) {
	dir := filepath.Join(workDir, PerTaskPluginCacheDirName)
	if fresh {
		if err := os.RemoveAll(dir); err != nil {
			return "", fmt.Errorf("reset plugin cache %s: %w", dir, err)
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create plugin cache %s: %w", dir, err)
	}
	return dir, nil
}

// withPluginCache returns env without any protected key, plus
// TF_PLUGIN_CACHE_DIR=cacheDir when cacheDir is set (no cache otherwise:
// never fall back to a shared one).
func withPluginCache(env []string, cacheDir string) []string {
	env = sanitizeTerraformEnv(env)
	if cacheDir != "" {
		env = append(env, "TF_PLUGIN_CACHE_DIR="+cacheDir)
	}
	return env
}

// terraformInitArgs the init arguments: never -upgrade (the lock file is
// authoritative). noStateLock adds -lock=false (plan-only tasks on the HTTP
// state backend).
func terraformInitArgs(noStateLock bool) []string {
	args := []string{"init", "-no-color", "-input=false"}
	if noStateLock {
		args = append(args, "-lock=false")
	}
	return args
}

// TaskErrorCode maps a task failure to its structured code
// (workspace_tasks.error_code), "" when there is none.
func TaskErrorCode(err error) string {
	var rr *manifestbundle.RepublishRequiredError
	if errors.As(err, &rr) {
		return models.TaskErrorCodeBundleRepublishRequired
	}
	// executor-side (agent or local) hand-off integrity failure: the bundle
	// received did not hash to bundle_hash (the platform verified the stored
	// files before handing them out)
	if errors.Is(err, manifestbundle.ErrIntegrity) {
		return models.TaskErrorCodeBundleHashMismatch
	}
	if errors.Is(err, crypto.ErrPlanDataExpired) || errors.Is(err, ErrPlanDataMissing) {
		return models.TaskErrorCodePlanExpired
	}
	return ""
}

// TaskErrorReason the short machine reason next to TaskErrorCode
// (workspace_tasks.error_reason): a rule name only, "" when there is none.
func TaskErrorReason(err error) string {
	var rr *manifestbundle.RepublishRequiredError
	if errors.As(err, &rr) {
		return rr.Rule()
	}
	switch {
	case errors.Is(err, manifestbundle.ErrIntegrity):
		return manifestbundle.ReasonHashMismatch
	case errors.Is(err, crypto.ErrPlanDataExpired):
		return "plan_data_expired"
	case errors.Is(err, ErrPlanDataMissing):
		return "plan_data_missing"
	}
	return ""
}

// classifyTaskFailure returns the structured code and reason of a task
// failure and the message to store. A bundle-gate failure has no Terraform
// output to extract from, so its message is exactly
// "bundle_republish_required: <reason>".
func classifyTaskFailure(err error, message string) (code, reason, msg string) {
	code = TaskErrorCode(err)
	reason = TaskErrorReason(err)
	var rr *manifestbundle.RepublishRequiredError
	if errors.As(err, &rr) {
		return code, reason, rr.Error()
	}
	if errors.Is(err, manifestbundle.ErrIntegrity) {
		return code, reason, models.TaskErrorCodeBundleHashMismatch + ": " + manifestbundle.ReasonHashMismatch + " (" + err.Error() + ")"
	}
	if code == models.TaskErrorCodePlanExpired {
		return code, reason, models.TaskErrorCodePlanExpired + ": " + err.Error()
	}
	return code, reason, message
}
