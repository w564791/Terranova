// Package manifestbundle defines the immutable manifest bundle identity.
//
// A bundle is the set of files of one published manifest version (native: the
// manifest_files snapshot written at publish; git: the tree at a pinned commit
// SHA). Every consumer (backfill migration, publish, runs, approval/apply
// verification) must derive bundle_hash through Hash so the value is
// comparable across all of them.
package manifestbundle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"

	"gorm.io/gorm"
)

// HashFormatVersion is mixed into every hash. Changing the encoding below
// requires a new version string (and new hashes), never an in-place edit.
// v2 added the normalized file mode to every entry (v1 hashed path + content
// only and was never relied on by a released build; migration
// 20261004_04_manifest_bundle_v2 verifies each stored v1 hash and rewrites
// it as v2).
const HashFormatVersion = "terranova-bundle-v2"

// Normalized file modes. Only the executable bit is significant.
const (
	ModeRegular    = 0o644
	ModeExecutable = 0o755
)

// NormalizeMode maps a Unix permission/mode value (manifest_files.mode, e.g.
// 420 = 0644; git modes parsed as octal, e.g. 0o100755) to 0755 when any
// executable bit is set and to 0644 otherwise (including 0 = unknown).
func NormalizeMode(mode int) int {
	if mode&0o111 != 0 {
		return ModeExecutable
	}
	return ModeRegular
}

// ModeFromGit parses a git tree entry mode ("100644", "100755"; legacy
// "100664") into a normalized mode. Only regular files are accepted:
// symlinks (120000), submodules (160000) and trees are not bundle files.
func ModeFromGit(mode string) (int, error) {
	m, err := strconv.ParseInt(mode, 8, 32)
	if err != nil || m&0o170000 != 0o100000 {
		return 0, fmt.Errorf("unsupported git file mode %q", mode)
	}
	return NormalizeMode(int(m)), nil
}

// File is one bundle entry: a POSIX relative path (as stored in
// manifest_files.path, no leading slash), its raw bytes and its mode
// (hashed after NormalizeMode).
//
// Mime / IsBinary are stored metadata carried through Pack and Store; they
// are not part of the hash.
type File struct {
	Path     string
	Content  []byte
	Mime     string
	IsBinary bool
	Mode     int
}

// Hash returns the lowercase hex SHA-256 bundle hash of files:
//
//	sha256( "terranova-bundle-v2" 0x00
//	        for each file sorted by path (byte order):
//	          path 0x00 mode 0x00 decimal(len(content)) 0x00 content )
//
// mode is the ASCII octal text of NormalizeMode(f.Mode): "644" or "755".
//
// The length prefix makes the encoding unambiguous for binary content that may
// itself contain NUL bytes; paths cannot contain NUL. Input order does not
// matter. Duplicate paths are rejected. An empty bundle has a well-defined
// hash. backend/migrations/add_manifest_sandbox_schema.sql reproduces this
// encoding in SQL for manual backfill (TestHashGoldenVector pins both).
func Hash(files []File) (string, error) {
	sorted := make([]File, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	h := sha256.New()
	h.Write([]byte(HashFormatVersion))
	h.Write([]byte{0})
	for i, f := range sorted {
		if f.Path == "" {
			return "", fmt.Errorf("bundle file with empty path")
		}
		if i > 0 && sorted[i-1].Path == f.Path {
			return "", fmt.Errorf("duplicate bundle path %q", f.Path)
		}
		h.Write([]byte(f.Path))
		h.Write([]byte{0})
		h.Write([]byte(strconv.FormatInt(int64(NormalizeMode(f.Mode)), 8)))
		h.Write([]byte{0})
		h.Write([]byte(strconv.Itoa(len(f.Content))))
		h.Write([]byte{0})
		h.Write(f.Content)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VersionHash computes the bundle hash of a published version from its
// manifest_files snapshot (version_id = versionID).
func VersionHash(ctx context.Context, db *gorm.DB, versionID string) (string, error) {
	var rows []struct {
		Path    string
		Content []byte
		Mode    int
	}
	if err := db.WithContext(ctx).Table("manifest_files").
		Select("path, content, mode").
		Where("version_id = ?", versionID).
		Find(&rows).Error; err != nil {
		return "", fmt.Errorf("load files of version %s: %w", versionID, err)
	}
	files := make([]File, len(rows))
	for i, r := range rows {
		files[i] = File{Path: r.Path, Content: r.Content, Mode: r.Mode}
	}
	return Hash(files)
}

// LegacyHashV1 is the retired terranova-bundle-v1 encoding (path + content,
// no mode), the encoding of the step-2 backfill (migration
// 20261004_02_manifest_sandbox_schema and its SQL patch). Migration
// 20261004_04_manifest_bundle_v2 checks stored v1 hashes with it: a match is
// rewritten as v2, anything else becomes a sticky hash_mismatch. Never use it
// for new hashes.
func LegacyHashV1(files []File) (string, error) {
	sorted := make([]File, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	h := sha256.New()
	h.Write([]byte("terranova-bundle-v1"))
	h.Write([]byte{0})
	for i, f := range sorted {
		if f.Path == "" {
			return "", fmt.Errorf("bundle file with empty path")
		}
		if i > 0 && sorted[i-1].Path == f.Path {
			return "", fmt.Errorf("duplicate bundle path %q", f.Path)
		}
		h.Write([]byte(f.Path))
		h.Write([]byte{0})
		h.Write([]byte(strconv.Itoa(len(f.Content))))
		h.Write([]byte{0})
		h.Write(f.Content)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VersionHashV1 is VersionHash in the legacy v1 encoding; only the step-2
// backfill (20261004_02) uses it, so its Go and SQL forms stay identical.
func VersionHashV1(ctx context.Context, db *gorm.DB, versionID string) (string, error) {
	var rows []struct {
		Path    string
		Content []byte
	}
	if err := db.WithContext(ctx).Table("manifest_files").
		Select("path, content").
		Where("version_id = ?", versionID).
		Find(&rows).Error; err != nil {
		return "", fmt.Errorf("load files of version %s: %w", versionID, err)
	}
	files := make([]File, len(rows))
	for i, r := range rows {
		files[i] = File{Path: r.Path, Content: r.Content}
	}
	return LegacyHashV1(files)
}
