package manifestbundle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"gorm.io/gorm"

	"iac-platform/internal/models"
)

// Source is the single read-files interface of a manifest bundle source.
// Publish packs a Source into a bundle (native: the publisher's draft; git:
// the tree at a pinned commit SHA, step 8). Downstream consumers never read a
// Source directly: they open the stored bundle with OpenVersion /
// OpenBundle; places that actually use a version (install, previews, upgrade
// target, runner hand-off) additionally call VerifyForUse.
type Source interface {
	ReadFiles(ctx context.Context) ([]File, error)
}

// NativeDraft is one user's draft of a native manifest
// (manifest_files.version_id IS NULL AND owner_user_id = user).
// Only the editor and publish read it.
type NativeDraft struct {
	DB          *gorm.DB
	ManifestID  string
	OwnerUserID string
}

func (s NativeDraft) ReadFiles(ctx context.Context) ([]File, error) {
	return readFileRows(s.DB.WithContext(ctx).
		Where("manifest_id = ? AND owner_user_id = ? AND version_id IS NULL", s.ManifestID, s.OwnerUserID))
}

// nativeVersion is the stored snapshot of a published version
// (manifest_files.version_id = version). Unverified; use OpenVersion.
type nativeVersion struct {
	DB         *gorm.DB
	ManifestID string // optional extra guard
	VersionID  string
}

func (s nativeVersion) ReadFiles(ctx context.Context) ([]File, error) {
	q := s.DB.WithContext(ctx).Where("version_id = ?", s.VersionID)
	if s.ManifestID != "" {
		q = q.Where("manifest_id = ?", s.ManifestID)
	}
	return readFileRows(q)
}

// ErrGitSourceNotImplemented git sources arrive in step 8.
var ErrGitSourceNotImplemented = errors.New("git manifest source is not implemented yet")

// GitCommit is the git source (repo tree at a pinned commit SHA). Stub until
// step 8: publish will pack it through the same Pack / Store path.
type GitCommit struct {
	RepoURL string
	Subpath string
	SHA     string
}

func (GitCommit) ReadFiles(context.Context) ([]File, error) {
	return nil, ErrGitSourceNotImplemented
}

func readFileRows(q *gorm.DB) ([]File, error) {
	var rows []models.ManifestFile
	if err := q.Model(&models.ManifestFile{}).
		Select("path, content, mime, is_binary, mode").
		Order("path").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("read manifest files: %w", err)
	}
	files := make([]File, len(rows))
	for i, r := range rows {
		files[i] = File{Path: r.Path, Content: r.Content, Mime: r.Mime, IsBinary: r.IsBinary, Mode: r.Mode}
	}
	return files, nil
}

// Bundle an immutable, content-addressed file set.
type Bundle struct {
	// Hash is the stored bundle_hash (verified only by Verify /
	// VerifyForUse). Empty when the version has no valid bundle (bundle_hash
	// NULL, see InvalidReason).
	Hash          string
	VersionID     string
	ManifestID    string
	InvalidReason string
	Files         []File
}

// Scope path -> content, the form the HCL parsers take.
func (b *Bundle) Scope() map[string][]byte {
	out := make(map[string][]byte, len(b.Files))
	for _, f := range b.Files {
		out[f.Path] = f.Content
	}
	return out
}

// InvalidError the version has no valid bundle (bundle_hash NULL); it must be
// republished. Reason holds only rule names and paths, or hash_mismatch.
type InvalidError struct {
	VersionID string
	Reason    string
}

func (e *InvalidError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("version %s has no valid bundle", e.VersionID)
	}
	return fmt.Sprintf("version %s has no valid bundle: %s", e.VersionID, e.Reason)
}

// RequireValid fails with *InvalidError when the version has no bundle_hash
// (no re-hash; see Verify / VerifyForUse). Deploy paths (install, upgrade,
// previews) call it; later the runner (step 4) and approval (step 7) do too.
func (b *Bundle) RequireValid() error {
	// a recorded reason wins over a stray hash (e.g. a manual re-run of the
	// step-2 SQL backfill, which has no reason guard)
	if b.Hash == "" || b.InvalidReason != "" {
		return &InvalidError{VersionID: b.VersionID, Reason: b.InvalidReason}
	}
	return nil
}

// ErrVersionNotFound no such version (in that manifest).
var ErrVersionNotFound = errors.New("manifest version not found")

// ErrIntegrity the stored files no longer hash to bundle_hash.
var ErrIntegrity = errors.New("manifest bundle integrity check failed")

// Pack reads a source and turns it into a bundle: every rule is checked
// (Validate) and the hash is manifestbundle.Hash. When problems is non-empty
// the bundle is nil and nothing may be stored.
func Pack(ctx context.Context, src Source) (b *Bundle, problems []Problem, err error) {
	files, err := src.ReadFiles(ctx)
	if err != nil {
		return nil, nil, err
	}
	return PackFiles(files)
}

// PackFiles is Pack for a file set already read from its Source.
func PackFiles(files []File) (b *Bundle, problems []Problem, err error) {
	if problems := Validate(files); len(problems) > 0 {
		return nil, problems, nil
	}
	hash, err := Hash(files)
	if err != nil {
		return nil, nil, err
	}
	return &Bundle{Hash: hash, Files: files}, nil, nil
}

// Store writes a packed bundle as the immutable snapshot of a version (the
// existing manifest_files version rows, owner NULL) and sets bundle_hash.
// It must run in the publish transaction, after the manifest_versions row is
// created. The stored rows are re-hashed with VersionHash so the value written
// is what every later reader will verify against.
func Store(ctx context.Context, tx *gorm.DB, manifestID, versionID string, b *Bundle) error {
	if len(b.Files) > 0 {
		rows := make([]models.ManifestFile, len(b.Files))
		for i, f := range b.Files {
			content := f.Content
			if content == nil {
				content = []byte{}
			}
			vid := versionID
			rows[i] = models.ManifestFile{
				ManifestID: manifestID, VersionID: &vid, Path: f.Path, Content: content,
				Mime: f.Mime, Size: len(content), IsBinary: f.IsBinary,
			}
			if rows[i].Mime == "" {
				rows[i].Mime = "application/octet-stream"
			}
			rows[i].Mode = NormalizeMode(f.Mode) // stored normalized; the hash only sees 0644 / 0755
		}
		if err := tx.WithContext(ctx).CreateInBatches(rows, 200).Error; err != nil {
			return fmt.Errorf("store bundle files: %w", err)
		}
	}
	stored, err := VersionHash(ctx, tx, versionID)
	if err != nil {
		return err
	}
	if stored != b.Hash {
		return fmt.Errorf("%w: stored %s, packed %s", ErrIntegrity, stored, b.Hash)
	}
	return tx.WithContext(ctx).Model(&models.ManifestVersion{}).Where("id = ?", versionID).
		Updates(map[string]interface{}{"bundle_hash": b.Hash, "bundle_invalid_reason": nil}).Error
}

// OpenVersion reads the stored bundle of a published version: its files plus
// the stored bundle_hash / bundle_invalid_reason. It never re-hashes and
// never writes, so read-only endpoints (list, detail, export, diff, workdirs)
// stay cheap and side-effect free. Places that actually use the version
// (install, previews, upgrade target, runner hand-off) call RequireValid
// and/or VerifyForUse. manifestID may be empty when the caller only has a
// version id.
func OpenVersion(ctx context.Context, db *gorm.DB, manifestID, versionID string) (*Bundle, error) {
	var v struct {
		ID                  string
		ManifestID          string
		BundleHash          *string
		BundleInvalidReason *string
	}
	q := db.WithContext(ctx).Model(&models.ManifestVersion{}).
		Select("id, manifest_id, bundle_hash, bundle_invalid_reason").Where("id = ?", versionID)
	if manifestID != "" {
		q = q.Where("manifest_id = ?", manifestID)
	}
	if err := q.Take(&v).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVersionNotFound
		}
		return nil, fmt.Errorf("load manifest version %s: %w", versionID, err)
	}
	return openStored(ctx, db, v.ManifestID, v.ID, v.BundleHash, v.BundleInvalidReason)
}

// OpenBundle content-addressed read: the bundle whose stored hash is
// bundleHash (any version carrying it). Unverified like OpenVersion.
func OpenBundle(ctx context.Context, db *gorm.DB, bundleHash string) (*Bundle, error) {
	var v struct {
		ID         string
		ManifestID string
	}
	if err := db.WithContext(ctx).Model(&models.ManifestVersion{}).
		Select("id, manifest_id").Where("bundle_hash = ?", bundleHash).
		Order("created_at, id").Take(&v).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVersionNotFound
		}
		return nil, fmt.Errorf("load bundle %s: %w", bundleHash, err)
	}
	h := bundleHash
	return openStored(ctx, db, v.ManifestID, v.ID, &h, nil)
}

func openStored(ctx context.Context, db *gorm.DB, manifestID, versionID string, bundleHash, reason *string) (*Bundle, error) {
	files, err := nativeVersion{DB: db, ManifestID: manifestID, VersionID: versionID}.ReadFiles(ctx)
	if err != nil {
		return nil, err
	}
	b := &Bundle{VersionID: versionID, ManifestID: manifestID, Files: files}
	if bundleHash != nil {
		b.Hash = *bundleHash
	}
	if reason != nil {
		b.InvalidReason = *reason
	}
	return b, nil
}

// Verify re-hashes the files and compares with the stored bundle_hash.
// *InvalidError when the version has no valid bundle; ErrIntegrity when the
// files no longer match. Pure: no DB access. Use VerifyForUse in request /
// runner paths so a mismatch is recorded and reported.
func (b *Bundle) Verify() error {
	if err := b.RequireValid(); err != nil {
		return err
	}
	got, err := Hash(b.Files)
	if err != nil || got != b.Hash {
		return fmt.Errorf("%w: version %s", ErrIntegrity, b.VersionID)
	}
	return nil
}

// ReasonHashMismatch is the sticky bundle_invalid_reason of a version whose
// stored files no longer match its bundle_hash. Once set, no recompute,
// backfill or migration may make the version valid again; only publishing a
// new version produces a valid bundle (for that new version).
const ReasonHashMismatch = "hash_mismatch"

// MismatchEvent identifies where a hash mismatch was detected.
type MismatchEvent struct {
	ManifestID string
	VersionID  string
	RequestID  string // HTTP request id, empty for background callers
	Source     string // e.g. "install", "upgrade_target", "runner"
	UserID     string
}

// securityLogf is the WARN sink of ReportHashMismatch (stdlib log, the
// repo's request logging; tests capture it with log.SetOutput).
var securityLogf = log.Printf

// ReportHashMismatch records a detected mismatch: best effort
// bundle_hash = NULL + bundle_invalid_reason = 'hash_mismatch' (a failure is
// logged, never returned), a WARN security log line and an audit_logs row
// (resource MANIFEST_VERSION, action version.bundle_hash_mismatch).
func ReportHashMismatch(ctx context.Context, db *gorm.DB, ev MismatchEvent) {
	securityLogf("[WARN] [security] manifest bundle hash mismatch: manifest_id=%s version_id=%s request_id=%s source=%s; version marked %s, republish required",
		ev.ManifestID, ev.VersionID, ev.RequestID, ev.Source, ReasonHashMismatch)
	if err := db.WithContext(ctx).Model(&models.ManifestVersion{}).Where("id = ?", ev.VersionID).
		Updates(map[string]interface{}{"bundle_hash": nil, "bundle_invalid_reason": ReasonHashMismatch}).Error; err != nil {
		securityLogf("[WARN] [security] could not record %s on manifest version %s (request_id=%s): %v", ReasonHashMismatch, ev.VersionID, ev.RequestID, err)
	}
	raw, _ := json.Marshal(map[string]string{
		"level": "WARN", "manifest_id": ev.ManifestID, "version_id": ev.VersionID,
		"request_id": ev.RequestID, "source": ev.Source, "reason": ReasonHashMismatch,
	})
	var uid *string
	if ev.UserID != "" {
		uid = &ev.UserID
	}
	if err := db.WithContext(ctx).Create(&models.AuditLog{
		UserID: uid, Action: "version.bundle_hash_mismatch", ResourceType: "MANIFEST_VERSION", NewValues: string(raw),
	}).Error; err != nil {
		securityLogf("[WARN] [security] could not write hash mismatch audit row for manifest version %s (request_id=%s): %v", ev.VersionID, ev.RequestID, err)
	}
}

// VerifyForUse is the integrity gate of places that actually use a version.
// A version already marked hash_mismatch is rejected without re-hashing.
// When the version has a bundle_hash the files are re-hashed; on mismatch
// the event is reported (ReportHashMismatch) and an *InvalidError with
// Reason hash_mismatch is returned. A version without bundle_hash for another
// reason (bundle rules) returns nil here: callers that require a valid bundle
// call RequireValid first.
func VerifyForUse(ctx context.Context, db *gorm.DB, b *Bundle, ev MismatchEvent) error {
	if b.InvalidReason == ReasonHashMismatch {
		return &InvalidError{VersionID: b.VersionID, Reason: ReasonHashMismatch}
	}
	if b.Hash == "" || b.InvalidReason != "" {
		return nil // no valid bundle for another reason (rules): RequireValid decides
	}
	if got, err := Hash(b.Files); err == nil && got == b.Hash {
		return nil
	}
	if ev.VersionID == "" {
		ev.VersionID = b.VersionID
	}
	if ev.ManifestID == "" {
		ev.ManifestID = b.ManifestID
	}
	ReportHashMismatch(ctx, db, ev)
	b.Hash, b.InvalidReason = "", ReasonHashMismatch
	return &InvalidError{VersionID: b.VersionID, Reason: ReasonHashMismatch}
}

// CheckStoredVersion is the migration check of one version against its
// stored bundle_hash. When storedHash is set it must equal the v2 or legacy
// v1 hash of the current files; otherwise mismatch is true and nothing else
// is computed (never re-derive a hash from content that no longer matches).
// Without a stored hash (or after a match) the bundle rules are applied and
// hash is the v2 hash when problems is empty.
func CheckStoredVersion(ctx context.Context, db *gorm.DB, versionID, storedHash string) (hash string, problems []Problem, mismatch bool, err error) {
	files, err := nativeVersion{DB: db, VersionID: versionID}.ReadFiles(ctx)
	if err != nil {
		return "", nil, false, err
	}
	v2, err := Hash(files)
	if err != nil {
		// duplicate / empty paths: reported through the rules below
		v2 = ""
	}
	if storedHash != "" && storedHash != v2 {
		if v1, err := LegacyHashV1(files); err != nil || v1 != storedHash {
			return "", nil, true, nil
		}
	}
	if problems := Validate(files); len(problems) > 0 {
		return "", problems, false, nil
	}
	return v2, nil, false, nil
}

// CheckVersion re-reads a version's stored files and applies the bundle
// rules: the recompute migration uses it. Files are never modified. hash is
// set only when problems is empty.
func CheckVersion(ctx context.Context, db *gorm.DB, versionID string) (hash string, problems []Problem, err error) {
	files, err := nativeVersion{DB: db, VersionID: versionID}.ReadFiles(ctx)
	if err != nil {
		return "", nil, err
	}
	if problems := Validate(files); len(problems) > 0 {
		return "", problems, nil
	}
	hash, err = Hash(files)
	return hash, nil, err
}

// RepublishRequiredError is the runner-side form of *InvalidError: its text
// is exactly "bundle_republish_required: <reason>" (rule names and paths, or
// hash_mismatch; never file content). The executor fails the task with it.
type RepublishRequiredError struct {
	Invalid *InvalidError
}

func (e *RepublishRequiredError) Error() string {
	reason := e.Invalid.Reason
	if reason == "" {
		reason = "no valid bundle"
	}
	return "bundle_republish_required: " + reason
}

func (e *RepublishRequiredError) Unwrap() error { return e.Invalid }

// RequireValidForRun is the runner hand-off gate: VerifyForUse (re-hash;
// a mismatch is recorded and reported) and then RequireValid, so a version
// whose bundle_hash is NULL for any reason (bundle rules or hash_mismatch)
// is never handed to Terraform. Failures are *RepublishRequiredError.
// recordDB receives the hash_mismatch record (pass a handle outside any
// surrounding transaction so a rollback cannot lose it).
func RequireValidForRun(ctx context.Context, recordDB *gorm.DB, b *Bundle, ev MismatchEvent) error {
	err := VerifyForUse(ctx, recordDB, b, ev)
	if err == nil {
		err = b.RequireValid()
	}
	var invalid *InvalidError
	if errors.As(err, &invalid) {
		return &RepublishRequiredError{Invalid: invalid}
	}
	return err
}
