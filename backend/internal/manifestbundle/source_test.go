package manifestbundle

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func sourceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`CREATE TABLE manifest_files (id INTEGER PRIMARY KEY AUTOINCREMENT, manifest_id TEXT, version_id TEXT, owner_user_id TEXT, path TEXT, content BLOB, mime TEXT, size INTEGER, is_binary INTEGER, mode INTEGER, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE manifest_versions (id TEXT PRIMARY KEY, manifest_id TEXT, version TEXT, bundle_hash TEXT, bundle_invalid_reason TEXT, created_at DATETIME)`,
		`INSERT INTO manifest_versions (id, manifest_id, version) VALUES ('mfv-1', 'mf-1', 'v1'), ('mfv-bad', 'mf-1', 'v0')`,
		`UPDATE manifest_versions SET bundle_invalid_reason = 'denylisted_file @ a.tfvars' WHERE id = 'mfv-bad'`,
		`INSERT INTO manifest_files (manifest_id, version_id, path, content) VALUES ('mf-1', 'mfv-bad', 'a.tfvars', X'00')`,
		`INSERT INTO manifest_files (manifest_id, version_id, owner_user_id, path, content) VALUES ('mf-1', NULL, 'u1', 'main.tf', CAST('draft' AS BLOB)), ('mf-1', NULL, 'u2', 'other.tf', CAST('x' AS BLOB))`,
	} {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("%v: %s", err, s)
		}
	}
	return db
}

func TestPackStoreOpen_RoundTrip(t *testing.T) {
	ctx := context.Background()
	db := sourceDB(t)
	b, problems, err := Pack(ctx, NativeDraft{DB: db, ManifestID: "mf-1", OwnerUserID: "u1"})
	if err != nil || problems != nil || len(b.Files) != 1 {
		t.Fatalf("Pack: %+v %v %v", b, problems, err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return Store(ctx, tx, "mf-1", "mfv-1", b) }); err != nil {
		t.Fatal(err)
	}
	got, err := OpenVersion(ctx, db, "mf-1", "mfv-1")
	if err != nil || got.Hash != b.Hash || string(got.Scope()["main.tf"]) != "draft" || got.RequireValid() != nil {
		t.Fatalf("OpenVersion: %+v %v", got, err)
	}
	if byHash, err := OpenBundle(ctx, db, b.Hash); err != nil || byHash.VersionID != "mfv-1" {
		t.Fatalf("OpenBundle: %+v %v", byHash, err)
	}
	if _, err := OpenBundle(ctx, db, "nope"); !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("OpenBundle unknown: %v", err)
	}
	// wrong manifest / unknown version
	if _, err := OpenVersion(ctx, db, "mf-other", "mfv-1"); !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("cross-manifest open: %v", err)
	}
	// draft edits after Store do not change the snapshot
	db.Exec(`UPDATE manifest_files SET content = CAST('edited' AS BLOB) WHERE version_id IS NULL`)
	if again, err := OpenVersion(ctx, db, "", "mfv-1"); err != nil || string(again.Scope()["main.tf"]) != "draft" {
		t.Fatalf("snapshot changed: %+v %v", again, err)
	}
	// tampering: OpenVersion stays a plain read (stored hash, no re-hash),
	// Verify detects it
	db.Exec(`UPDATE manifest_files SET content = CAST('evil' AS BLOB) WHERE version_id = 'mfv-1'`)
	tampered, err := OpenVersion(ctx, db, "", "mfv-1")
	if err != nil || tampered.Hash != b.Hash {
		t.Fatalf("OpenVersion must not verify: %+v %v", tampered, err)
	}
	if err := tampered.Verify(); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("tamper: %v", err)
	}
}

func TestOpenVersion_NullHashIsLenientButNotValid(t *testing.T) {
	b, err := OpenVersion(context.Background(), sourceDB(t), "mf-1", "mfv-bad")
	if err != nil || b.Hash != "" || len(b.Files) != 1 {
		t.Fatalf("OpenVersion: %+v %v", b, err)
	}
	var inv *InvalidError
	if err := b.RequireValid(); !errors.As(err, &inv) || inv.Reason != "denylisted_file @ a.tfvars" {
		t.Fatalf("RequireValid: %v", err)
	}
}

func TestPack_ReportsProblemsAndStoresNothing(t *testing.T) {
	b, problems, err := PackFiles([]File{f("main.tf", "x"), f("prod.tfvars", "x")})
	if err != nil || b != nil || len(problems) != 1 || problems[0].Rule != RuleDenylistedFile {
		t.Fatalf("PackFiles: %+v %v %v", b, problems, err)
	}
	hash, problems, err := CheckVersion(context.Background(), sourceDB(t), "mfv-bad")
	if err != nil || hash != "" || len(problems) != 1 {
		t.Fatalf("CheckVersion: %q %v %v", hash, problems, err)
	}
}

func TestGitCommitSource(t *testing.T) {
	files := []File{{Path: "main.tf", Content: []byte("# x\n"), Mode: ModeRegular}}
	b, probs, err := Pack(context.Background(), GitCommit{SHA: "0123456789abcdef0123456789abcdef01234567", Files: files})
	if err != nil || len(probs) != 0 {
		t.Fatalf("pack: %v %v", err, probs)
	}
	if want, _ := Hash(files); b.Hash != want {
		t.Fatalf("hash = %s, want %s", b.Hash, want)
	}
}

func TestStore_ModeIsNormalizedHashedAndVerified(t *testing.T) {
	ctx := context.Background()
	db := sourceDB(t)
	db.Exec(`INSERT INTO manifest_files (manifest_id, version_id, owner_user_id, path, content, mode) VALUES ('mf-1', NULL, 'u1', 'run.sh', CAST('echo' AS BLOB), 509)`)
	b, problems, err := Pack(ctx, NativeDraft{DB: db, ManifestID: "mf-1", OwnerUserID: "u1"})
	if err != nil || problems != nil {
		t.Fatalf("Pack: %v %v", problems, err)
	}
	flat := make([]File, len(b.Files))
	for i, f := range b.Files {
		flat[i] = File{Path: f.Path, Content: f.Content, Mode: ModeRegular}
	}
	if h, _ := Hash(flat); h == b.Hash {
		t.Fatal("executable bit must be part of the bundle hash")
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return Store(ctx, tx, "mf-1", "mfv-1", b) }); err != nil {
		t.Fatal(err)
	}
	var mode int
	db.Raw(`SELECT mode FROM manifest_files WHERE version_id = 'mfv-1' AND path = 'run.sh'`).Scan(&mode)
	if mode != ModeExecutable {
		t.Fatalf("stored mode %o, want 755", mode)
	}
	// flipping only the stored mode is detected like a content change
	db.Exec(`UPDATE manifest_files SET mode = 420 WHERE version_id = 'mfv-1' AND path = 'run.sh'`)
	if tampered, _ := OpenVersion(ctx, db, "", "mfv-1"); !errors.Is(tampered.Verify(), ErrIntegrity) {
		t.Fatal("mode tamper must fail Verify")
	}
}

func TestVerifyForUse_MarksReportsAndIsSticky(t *testing.T) {
	ctx := context.Background()
	db := sourceDB(t)
	db.Exec(`CREATE TABLE audit_logs (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT, action TEXT, resource_type TEXT, resource_id INTEGER, old_values TEXT, new_values TEXT, ip_address TEXT, user_agent TEXT, created_at DATETIME, deleted_at DATETIME)`)
	b, _, _ := Pack(ctx, NativeDraft{DB: db, ManifestID: "mf-1", OwnerUserID: "u1"})
	if err := db.Transaction(func(tx *gorm.DB) error { return Store(ctx, tx, "mf-1", "mfv-1", b) }); err != nil {
		t.Fatal(err)
	}
	var logs []string
	securityLogf = func(format string, args ...interface{}) { logs = append(logs, fmt.Sprintf(format, args...)) }
	t.Cleanup(func() { securityLogf = log.Printf })
	ev := MismatchEvent{RequestID: "req-12345678", Source: "install"}

	ok, _ := OpenVersion(ctx, db, "mf-1", "mfv-1")
	if err := VerifyForUse(ctx, db, ok, ev); err != nil || len(logs) != 0 {
		t.Fatalf("intact bundle: %v %v", err, logs)
	}

	db.Exec(`UPDATE manifest_files SET content = CAST('evil' AS BLOB) WHERE version_id = 'mfv-1'`)
	bad, _ := OpenVersion(ctx, db, "mf-1", "mfv-1")
	var inv *InvalidError
	if err := VerifyForUse(ctx, db, bad, ev); !errors.As(err, &inv) || inv.Reason != ReasonHashMismatch {
		t.Fatalf("want hash_mismatch InvalidError, got %v", err)
	}
	if len(logs) != 1 || !strings.HasPrefix(logs[0], "[WARN] [security]") ||
		!strings.Contains(logs[0], "manifest_id=mf-1") || !strings.Contains(logs[0], "version_id=mfv-1") || !strings.Contains(logs[0], "request_id=req-12345678") {
		t.Fatalf("WARN log = %v", logs)
	}
	var row struct {
		Action, ResourceType, NewValues string
	}
	db.Raw(`SELECT action, resource_type, new_values FROM audit_logs`).Scan(&row)
	if row.Action != "version.bundle_hash_mismatch" || row.ResourceType != "MANIFEST_VERSION" ||
		!strings.Contains(row.NewValues, `"level":"WARN"`) || !strings.Contains(row.NewValues, `"request_id":"req-12345678"`) ||
		!strings.Contains(row.NewValues, `"version_id":"mfv-1"`) || !strings.Contains(row.NewValues, `"manifest_id":"mf-1"`) {
		t.Fatalf("audit row = %+v", row)
	}
	var v struct {
		BundleHash          *string
		BundleInvalidReason *string
	}
	db.Raw(`SELECT bundle_hash, bundle_invalid_reason FROM manifest_versions WHERE id = 'mfv-1'`).Scan(&v)
	if v.BundleHash != nil || v.BundleInvalidReason == nil || *v.BundleInvalidReason != ReasonHashMismatch {
		t.Fatalf("version not marked: %+v", v)
	}

	// sticky: content restored and even a stray hash written back (e.g. a manual
	// re-run of the step-2 SQL backfill) => still rejected, no re-hash, no new report
	db.Exec(`UPDATE manifest_files SET content = CAST('draft' AS BLOB) WHERE version_id = 'mfv-1'`)
	db.Exec(`UPDATE manifest_versions SET bundle_hash = ? WHERE id = 'mfv-1'`, b.Hash)
	again, _ := OpenVersion(ctx, db, "mf-1", "mfv-1")
	if err := again.RequireValid(); !errors.As(err, &inv) || inv.Reason != ReasonHashMismatch {
		t.Fatalf("RequireValid must honor the recorded reason: %v", err)
	}
	if err := VerifyForUse(ctx, db, again, ev); !errors.As(err, &inv) || inv.Reason != ReasonHashMismatch || len(logs) != 1 {
		t.Fatalf("hash_mismatch must be sticky: %v, logs %d", err, len(logs))
	}
}

func TestVerifyForUse_RuleFailureIsNotAMismatch(t *testing.T) {
	ctx := context.Background()
	db := sourceDB(t) // mfv-bad: NULL hash + rules reason
	var logs []string
	securityLogf = func(format string, args ...interface{}) { logs = append(logs, fmt.Sprintf(format, args...)) }
	t.Cleanup(func() { securityLogf = log.Printf })
	b, _ := OpenVersion(ctx, db, "mf-1", "mfv-bad")
	if err := VerifyForUse(ctx, db, b, MismatchEvent{Source: "runner"}); err != nil || len(logs) != 0 {
		t.Fatalf("rule failure must not be reported as a mismatch: %v %v", err, logs)
	}
	if err := b.RequireValid(); err == nil {
		t.Fatal("rule failure must still be invalid for deploy paths")
	}
}

func TestCheckStoredVersion_V1V2AndMismatch(t *testing.T) {
	ctx := context.Background()
	db := sourceDB(t)
	db.Exec(`INSERT INTO manifest_files (manifest_id, version_id, path, content, mode) VALUES ('mf-1', 'mfv-1', 'main.tf', CAST('x' AS BLOB), 420)`)
	files := []File{{Path: "main.tf", Content: []byte("x"), Mode: 420}}
	v1, _ := LegacyHashV1(files)
	v2, _ := Hash(files)
	for name, stored := range map[string]string{"v1": v1, "v2": v2, "none": ""} {
		if h, _, mismatch, err := CheckStoredVersion(ctx, db, "mfv-1", stored); err != nil || mismatch || h != v2 {
			t.Fatalf("%s: %q %v %v", name, h, mismatch, err)
		}
	}
	if h, _, mismatch, _ := CheckStoredVersion(ctx, db, "mfv-1", strings.Repeat("0", 64)); !mismatch || h != "" {
		t.Fatalf("foreign hash must be a mismatch: %q %v", h, mismatch)
	}
}
