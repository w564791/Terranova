package services

import (
	"archive/zip"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"iac-platform/internal/manifestbundle"
	"iac-platform/internal/models"
)

func envValues(env []string, key string) []string {
	var out []string
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			out = append(out, v)
		}
	}
	return out
}

func TestTerraformInitArgs_NeverUpgrade(t *testing.T) {
	for _, noLock := range []bool{false, true} {
		args := terraformInitArgs(noLock)
		if args[0] != "init" {
			t.Fatalf("%v", args)
		}
		for _, a := range args {
			if strings.Contains(a, "upgrade") {
				t.Fatalf("init must never upgrade: %v", args)
			}
		}
		if hasLock := strings.Contains(strings.Join(args, " "), "-lock=false"); hasLock != noLock {
			t.Fatalf("-lock=false mismatch: %v", args)
		}
	}
}

func TestWithPluginCache_PerTaskOnlyAndProtectedKeysDropped(t *testing.T) {
	work := t.TempDir()
	cache, err := preparePerTaskPluginCache(work, true)
	if err != nil {
		t.Fatal(err)
	}
	inherited := []string{
		"PATH=/usr/bin",
		"TF_PLUGIN_CACHE_DIR=/tmp/.terraform.d/plugin-cache", // agent pool shared cache
		"TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE=1",
		"TF_CLI_ARGS_init=-upgrade",
		"AWS_REGION=eu-west-1",
	}
	env := withPluginCache(inherited, cache)
	if got := envValues(env, "TF_PLUGIN_CACHE_DIR"); len(got) != 1 || got[0] != cache {
		t.Fatalf("TF_PLUGIN_CACHE_DIR must be exactly the per-task cache, got %v", got)
	}
	if !strings.HasPrefix(cache, work+string(os.PathSeparator)) {
		t.Fatalf("cache %s is not inside the task work dir %s", cache, work)
	}
	for _, k := range []string{"TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE", "TF_CLI_ARGS_init", "TF_CLI_CONFIG_FILE"} {
		if v := envValues(env, k); len(v) != 0 {
			t.Fatalf("%s must not be set: %v", k, v)
		}
	}
	if v := envValues(env, "AWS_REGION"); len(v) != 1 {
		t.Fatalf("unrelated variables must be kept: %v", env)
	}
	// no cache dir => no cache at all (never fall back to a shared one)
	if v := envValues(withPluginCache(inherited, ""), "TF_PLUGIN_CACHE_DIR"); len(v) != 0 {
		t.Fatalf("fallback to shared cache: %v", v)
	}
	for _, k := range []string{"TF_PLUGIN_CACHE_DIR", "TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE", "TF_CLI_ARGS_init"} {
		if !isProtectedTerraformEnvKey(k) {
			t.Fatalf("%s must be protected (workspace variables are filtered with it)", k)
		}
	}
}

func TestPreparePerTaskPluginCache_FreshWipesPlantedEntries(t *testing.T) {
	work := t.TempDir()
	planted := filepath.Join(work, PerTaskPluginCacheDirName, "registry.terraform.io", "hashicorp", "aws", "5.0.0", "linux_amd64", "terraform-provider-aws_v5.0.0")
	if err := os.MkdirAll(filepath.Dir(planted), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(planted, []byte("evil"), 0o755)
	dir, err := preparePerTaskPluginCache(work, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(planted); !os.IsNotExist(err) {
		t.Fatal("a fresh per-task cache must not contain pre-planted providers")
	}
	if fi, _ := os.Stat(dir); fi.Mode().Perm() != 0o700 {
		t.Fatalf("cache mode %v", fi.Mode().Perm())
	}
	// retry keeps the task's own downloads
	os.WriteFile(filepath.Join(dir, "own"), []byte("x"), 0o600)
	if _, err := preparePerTaskPluginCache(work, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "own")); err != nil {
		t.Fatal("retry must reuse the task's cache")
	}
}

func TestIsLockedProviderError_ChecksumAndVersion(t *testing.T) {
	s := &TerraformExecutor{}
	for _, msg := range []string{
		"the cached package for example.com/test/fake 1.0.0 (in .terraform/providers) does not match any of the checksums recorded in the dependency lock file",
		"Error while installing example.com/test/fake v1.0.0: the local package for example.com/test/fake 1.0.0 doesn't match any of the checksums previously recorded in the dependency lock file",
		"locked provider registry.terraform.io/hashicorp/aws 5.0.0 does not match configured version constraint ~> 6.0; must use terraform init -upgrade",
	} {
		if !s.isLockedProviderError(errors.New(msg)) {
			t.Fatalf("not classified as a lock mismatch: %s", msg)
		}
	}
	if s.isLockedProviderError(errors.New("i/o timeout")) {
		t.Fatal("network error classified as lock mismatch")
	}
}

func TestProviderConfigChanged(t *testing.T) {
	for _, c := range []struct {
		ws   models.Workspace
		want bool
	}{
		{models.Workspace{}, false},
		{models.Workspace{ProviderConfigHash: "a"}, false},                                             // first run
		{models.Workspace{ProviderConfigHash: "a", LastInitHash: "a"}, false},                          // unchanged
		{models.Workspace{ProviderConfigHash: "a", LastInitHash: "a", TerraformVersion: "1.9"}, false}, // tf version only
		{models.Workspace{ProviderConfigHash: "b", LastInitHash: "a"}, true},
	} {
		ws := c.ws
		if got := providerConfigChanged(&ws); got != c.want {
			t.Fatalf("%+v: %v", c.ws, got)
		}
	}
}

type lockOnlyAccessor struct {
	DataAccessor
	lock  string
	calls int
}

func (a *lockOnlyAccessor) GetTerraformLockHCL(string) (string, error) {
	a.calls++
	return a.lock, nil
}

func TestRestoreTerraformLockHCL_BundleLockWinsAndConfigChangeReResolves(t *testing.T) {
	logger := NewTerraformLogger(nil)
	acc := &lockOnlyAccessor{lock: "# stored lock\n"}
	s := &TerraformExecutor{dataAccessor: acc}

	// bundle-provided lock is kept as is
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".terraform.lock.hcl"), []byte("# bundle lock\n"), 0o644)
	s.restoreTerraformLockHCL(dir, &models.Workspace{WorkspaceID: "ws-1"}, logger)
	if b, _ := os.ReadFile(filepath.Join(dir, ".terraform.lock.hcl")); string(b) != "# bundle lock\n" || acc.calls != 0 {
		t.Fatalf("bundle lock overwritten: %q (calls %d)", b, acc.calls)
	}
	// provider configuration changed: stored lock not restored
	dir = t.TempDir()
	s.restoreTerraformLockHCL(dir, &models.Workspace{WorkspaceID: "ws-1", ProviderConfigHash: "b", LastInitHash: "a"}, logger)
	if _, err := os.Stat(filepath.Join(dir, ".terraform.lock.hcl")); !os.IsNotExist(err) {
		t.Fatal("stored lock restored after a provider configuration change")
	}
	// otherwise the stored lock is restored
	dir = t.TempDir()
	s.restoreTerraformLockHCL(dir, &models.Workspace{WorkspaceID: "ws-1", ProviderConfigHash: "a", LastInitHash: "a"}, logger)
	if b, _ := os.ReadFile(filepath.Join(dir, ".terraform.lock.hcl")); string(b) != "# stored lock\n" {
		t.Fatalf("stored lock not restored: %q", b)
	}
}

func TestClassifyTaskFailure_BundleRepublishRequired(t *testing.T) {
	gate := &manifestbundle.RepublishRequiredError{Invalid: &manifestbundle.InvalidError{VersionID: "v", Reason: "denylisted_file @ prod.tfvars"}}
	err := fmt.Errorf("failed to write manifest files: %w", fmt.Errorf("load manifest files: %w", gate))
	code, reason, msg := classifyTaskFailure(err, "extracted")
	if code != models.TaskErrorCodeBundleRepublishRequired || reason != "denylisted_file" || msg != "bundle_republish_required: denylisted_file @ prod.tfvars" {
		t.Fatalf("%q %q %q", code, reason, msg)
	}
	if code, reason, msg := classifyTaskFailure(errors.New("terraform plan failed"), "extracted"); code != "" || reason != "" || msg != "extracted" {
		t.Fatalf("%q %q %q", code, reason, msg)
	}
	// error_reason: rule name only, never paths / content
	for in, want := range map[string]string{
		"hash_mismatch": "hash_mismatch",
		"":              "no_valid_bundle",
		"secret_detected @ a/b.tf; denylisted_file @ x": "secret_detected",
		"Weird Reason / path":                           "invalid_bundle",
	} {
		rr := &manifestbundle.RepublishRequiredError{Invalid: &manifestbundle.InvalidError{Reason: in}}
		if got := TaskErrorReason(fmt.Errorf("wrap: %w", rr)); got != want {
			t.Errorf("reason %q => %q, want %q", in, got, want)
		}
	}
	if !models.KnownTaskErrorCode(models.TaskErrorCodeBundleRepublishRequired) || models.KnownTaskErrorCode("anything") {
		t.Fatal("KnownTaskErrorCode")
	}
}

// ---------------------------------------------------------------------------
// Real Terraform: lock file + per-task cache behaviour. Needs a terraform
// binary (>= 1.4): $TERRANOVA_TEST_TERRAFORM or `terraform` on PATH; skipped
// otherwise. Offline: a filesystem mirror with a fake provider package (init
// installs providers but never executes them). The test-only CLI config
// (TF_CLI_CONFIG_FILE) just points provider installation at the mirror.
// ---------------------------------------------------------------------------

func terraformBinary(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("TERRANOVA_TEST_TERRAFORM"); p != "" {
		return p
	}
	p, err := exec.LookPath("terraform")
	if err != nil {
		t.Skip("terraform binary not available (set TERRANOVA_TEST_TERRAFORM)")
	}
	return p
}

const fakeProviderMain = `terraform {
  required_providers {
    fake = {
      source  = "example.com/test/fake"
      version = "1.0.0"
    }
  }
}
`

type tfFixture struct {
	t        *testing.T
	bin      string
	mirror   string
	cliCfg   string
	platform string
}

func newTFFixture(t *testing.T) *tfFixture {
	f := &tfFixture{t: t, bin: terraformBinary(t), mirror: t.TempDir(), platform: runtime.GOOS + "_" + runtime.GOARCH}
	f.cliCfg = filepath.Join(t.TempDir(), "cli.tfrc")
	os.WriteFile(f.cliCfg, []byte(fmt.Sprintf("provider_installation {\n  filesystem_mirror {\n    path = %q\n  }\n}\n", f.mirror)), 0o644)
	return f
}

// writeMirror (re)writes the packed provider package with the given binary content.
func (f *tfFixture) writeMirror(content string) {
	dir := filepath.Join(f.mirror, "example.com", "test", "fake")
	os.RemoveAll(dir)
	os.MkdirAll(dir, 0o755)
	out, err := os.Create(filepath.Join(dir, "terraform-provider-fake_1.0.0_"+f.platform+".zip"))
	if err != nil {
		f.t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	hdr := &zip.FileHeader{Name: "terraform-provider-fake_v1.0.0", Method: zip.Deflate}
	hdr.SetMode(0o755)
	w, _ := zw.CreateHeader(hdr)
	w.Write([]byte(content))
	zw.Close()
	out.Close()
}

// init runs `terraform init` in a fresh task work dir the way the executor
// does (terraformInitArgs + per-task cache via withPluginCache), with the
// given inherited environment. lock, when set, is placed in the dir first.
func (f *tfFixture) init(lock string, inherited []string, plantCache string) (workDir, output string, err error) {
	workDir = f.t.TempDir()
	os.WriteFile(filepath.Join(workDir, "main.tf"), []byte(fakeProviderMain), 0o644)
	if lock != "" {
		os.WriteFile(filepath.Join(workDir, ".terraform.lock.hcl"), []byte(lock), 0o644)
	}
	cache, cerr := preparePerTaskPluginCache(workDir, true)
	if cerr != nil {
		f.t.Fatal(cerr)
	}
	if plantCache != "" {
		// simulates a poisoned cache entry inside the cache this init uses
		p := filepath.Join(cache, "example.com", "test", "fake", "1.0.0", f.platform, "terraform-provider-fake_v1.0.0")
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(plantCache), 0o755)
	}
	env := withPluginCache(append(inherited, "TF_CLI_CONFIG_FILE="+f.cliCfg, "HOME="+f.t.TempDir(), "PATH="+os.Getenv("PATH")), cache)
	cmd := exec.Command(f.bin, terraformInitArgs(false)...)
	cmd.Dir = workDir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return workDir, string(out), err
}

func installedProvider(t *testing.T, workDir, platform string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(workDir, ".terraform", "providers", "example.com", "test", "fake", "1.0.0", platform, "terraform-provider-fake_v1.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTerraformInit_RealBinary_LockAndPerTaskCache(t *testing.T) {
	f := newTFFixture(t)
	f.writeMirror("good-provider")

	// first init records the lock (h1 + zh hashes of the good package)
	dir, out, err := f.init("", nil, "")
	if err != nil {
		t.Fatalf("first init: %v\n%s", err, out)
	}
	lockBytes, err := os.ReadFile(filepath.Join(dir, ".terraform.lock.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	lock := string(lockBytes)

	// 1. tampered cache entry (hash differs from the lock) and no clean source:
	//    init must fail, never link the cached binary
	os.RemoveAll(filepath.Join(f.mirror, "example.com"))
	_, out, err = f.init(lock, nil, "evil-provider")
	if err == nil {
		t.Fatalf("init succeeded with a tampered cached provider:\n%s", out)
	}

	// 2. tampered package at the source: checksum mismatch against the lock fails init,
	//    classified as a lock mismatch (not retried, never -upgrade)
	f.writeMirror("evil-provider")
	_, out, err = f.init(lock, nil, "")
	if err == nil {
		t.Fatalf("init accepted a package that does not match the lock:\n%s", out)
	}
	if !(&TerraformExecutor{}).isLockedProviderError(errors.New(out)) {
		t.Fatalf("checksum mismatch not classified as lock mismatch:\n%s", out)
	}

	// 3. a poisoned shared cache from the inherited environment is ignored:
	//    the per-task cache is used and the good package is installed
	f.writeMirror("good-provider")
	shared := t.TempDir()
	sp := filepath.Join(shared, "example.com", "test", "fake", "1.0.0", f.platform, "terraform-provider-fake_v1.0.0")
	os.MkdirAll(filepath.Dir(sp), 0o755)
	os.WriteFile(sp, []byte("evil-provider"), 0o755)
	dir, out, err = f.init(lock, []string{"TF_PLUGIN_CACHE_DIR=" + shared, "TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE=1"}, "")
	if err != nil {
		t.Fatalf("init with poisoned shared cache in env: %v\n%s", err, out)
	}
	if got := installedProvider(t, dir, f.platform); got != "good-provider" {
		t.Fatalf("installed provider %q, want the lock-verified package", got)
	}

	// 4. tampered entry in the task's own cache but a clean source: the cached
	//    entry is not trusted (checksum differs from the lock), the good package wins
	dir, out, err = f.init(lock, nil, "evil-provider")
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	if got := installedProvider(t, dir, f.platform); got != "good-provider" {
		t.Fatalf("tampered cache entry was used: %q", got)
	}
}
