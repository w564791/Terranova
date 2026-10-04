package manifestbundle

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"iac-platform/internal/models"
)

// Source is the single read-files interface of a manifest bundle source.
// Publish packs a Source into a bundle (native: the publisher's draft; git:
// the tree at a pinned commit SHA, step 8). Downstream consumers never read a
// Source directly: they open the stored, hash-verified bundle with
// OpenVersion / OpenBundle.
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
	// Hash is the verified bundle_hash. Empty when the version has no valid
	// bundle (bundle_hash NULL, see InvalidReason).
	Hash          string
	VersionID     string
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
// republished. Reason holds only rule names and paths.
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

// RequireValid fails with *InvalidError when the bundle is not valid. Deploy
// paths (install, upgrade, previews) call it; later the runner (step 4) and
// approval (step 7) do too.
func (b *Bundle) RequireValid() error {
	if b.Hash == "" {
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

// OpenVersion opens the stored bundle of a published version. When the
// version has a bundle_hash the files are re-hashed and must match
// (ErrIntegrity otherwise). A version without bundle_hash (failed the bundle
// rules, or published by an old binary during a rolling upgrade) opens with
// Hash == "" so read-only consumers (export, diff, running workspaces) keep
// working; deploy paths call RequireValid. manifestID may be empty when the
// caller only has a version id.
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

// OpenBundle content-addressed read: the bundle whose hash is bundleHash
// (any version carrying it; identical hash means identical files).
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
	b := &Bundle{VersionID: versionID, Files: files}
	if bundleHash == nil || *bundleHash == "" {
		if reason != nil {
			b.InvalidReason = *reason
		}
		return b, nil
	}
	got, err := Hash(files)
	if err != nil || got != *bundleHash {
		return nil, fmt.Errorf("%w: version %s", ErrIntegrity, versionID)
	}
	b.Hash = got
	return b, nil
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
