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
const HashFormatVersion = "terranova-bundle-v1"

// File is one bundle entry: a POSIX relative path (as stored in
// manifest_files.path, no leading slash) and its raw bytes.
type File struct {
	Path    string
	Content []byte
}

// Hash returns the lowercase hex SHA-256 bundle hash of files:
//
//	sha256( "terranova-bundle-v1" 0x00
//	        for each file sorted by path (byte order):
//	          path 0x00 decimal(len(content)) 0x00 content )
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
	return Hash(files)
}
