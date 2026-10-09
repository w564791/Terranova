package manifestbundle

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Bundle hand-off archive.
//
// A runner never receives "a list of files to write somewhere": it receives
// one archive plus the bundle_hash the platform verified (RequireValidForRun)
// and unpacks it with Unpack, which re-checks every entry and the hash before
// anything else (terraform init included) touches the directory. Local runs,
// agent / K8s runs and the step-6 sandbox use the same archive and the same
// Unpack, so the integrity rules cannot drift between them.
//
// The archive is an uncompressed tar of regular files only, sorted by path,
// with normalized modes (0644 / 0755), zero mtime and uid/gid 0: the same
// bundle always produces the same bytes.

// Limits are the cumulative caps Unpack enforces while reading. Each check
// runs on the entry header, before the entry body is read, so an oversized or
// overlong archive is rejected as soon as the offending entry appears.
type Limits struct {
	MaxFileSize   int64 // bytes per file
	MaxBundleSize int64 // sum of all file sizes
	MaxEntries    int   // files + directories
}

// DefaultLimits the bundle rules' limits (MaxFileSize / MaxBundleSize /
// MaxFiles). Directory entries count toward MaxEntries too.
func DefaultLimits() Limits {
	return Limits{MaxFileSize: MaxFileSize, MaxBundleSize: MaxBundleSize, MaxEntries: MaxFiles}
}

// ErrUnsafeEntry an archive entry that is not a plain relative regular file
// or directory (bad path, symlink, hardlink, device, fifo, ...), or that
// would be written through / over something already on disk.
var ErrUnsafeEntry = errors.New("unsafe bundle archive entry")

// ErrLimitExceeded an archive over MaxFileSize / MaxBundleSize / MaxEntries.
var ErrLimitExceeded = errors.New("bundle archive limit exceeded")

// WriteArchive writes files as a deterministic hand-off archive. Paths are
// checked with ValidatePath and must be unique.
func WriteArchive(w io.Writer, files []File) error {
	sorted := make([]File, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	tw := tar.NewWriter(w)
	for i, f := range sorted {
		if rule := ValidatePath(f.Path); rule != "" {
			return fmt.Errorf("%w: %s (%s)", ErrUnsafeEntry, f.Path, rule)
		}
		if i > 0 && sorted[i-1].Path == f.Path {
			return fmt.Errorf("duplicate bundle path %q", f.Path)
		}
		hdr := &tar.Header{
			Typeflag: tar.TypeReg,
			Name:     f.Path,
			Mode:     int64(NormalizeMode(f.Mode)),
			Size:     int64(len(f.Content)),
			ModTime:  time.Unix(0, 0).UTC(),
			Format:   tar.FormatPAX,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("write archive header %s: %w", f.Path, err)
		}
		if _, err := tw.Write(f.Content); err != nil {
			return fmt.Errorf("write archive entry %s: %w", f.Path, err)
		}
	}
	return tw.Close()
}

// ArchiveBytes is WriteArchive into memory.
func ArchiveBytes(files []File) ([]byte, error) {
	var buf bytes.Buffer
	if err := WriteArchive(&buf, files); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Unpack extracts a hand-off archive into dest (an existing directory, not a
// symlink) and returns the unpacked files. It fails, without reading further,
// on the first entry that:
//   - has a path ValidatePath rejects (absolute, "..", backslash, control
//     characters, too long, not NFC, ...);
//   - is anything but a regular file or a directory (symlinks, hardlinks,
//     character / block devices, fifos, sparse / GNU special entries);
//   - pushes the archive over lim (checked on the header, before the body
//     is read).
//
// Files are created with O_CREAT|O_EXCL|O_NOFOLLOW and parent directories are
// created component by component, refusing any existing non-directory (e.g. a
// symlink planted in dest), so an entry can neither overwrite an existing file
// nor be written through a link.
//
// After the last entry the unpacked set must hash (Hash) to expectedHash;
// otherwise the error wraps ErrIntegrity and the caller must not run anything
// in dest. An empty expectedHash (version without a valid bundle, bundle_hash
// NULL) is refused up front with *RepublishRequiredError: such a bundle is
// never handed to Terraform.
func Unpack(r io.Reader, dest, expectedHash string, lim Limits) ([]File, error) {
	if expectedHash == "" {
		return nil, &RepublishRequiredError{Invalid: &InvalidError{}}
	}
	if fi, err := os.Lstat(dest); err != nil {
		return nil, fmt.Errorf("unpack destination: %w", err)
	} else if !fi.IsDir() {
		return nil, fmt.Errorf("%w: destination %s is not a directory", ErrUnsafeEntry, dest)
	}

	tr := tar.NewReader(r)
	var (
		files   []File
		entries int
		total   int64
	)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read bundle archive: %w", err)
		}
		entries++
		if entries > lim.MaxEntries {
			return nil, fmt.Errorf("%w: more than %d entries", ErrLimitExceeded, lim.MaxEntries)
		}

		name := hdr.Name
		switch hdr.Typeflag {
		case tar.TypeDir:
			name = strings.TrimSuffix(name, "/")
			if rule := ValidatePath(name); rule != "" {
				return nil, fmt.Errorf("%w: %q (%s)", ErrUnsafeEntry, hdr.Name, rule)
			}
			if err := mkdirAllNoFollow(dest, name); err != nil {
				return nil, err
			}
			continue
		case tar.TypeReg, tar.TypeRegA: //nolint:staticcheck // TypeRegA: legacy regular file
		default:
			return nil, fmt.Errorf("%w: %q has type %q (only regular files and directories are allowed)",
				ErrUnsafeEntry, hdr.Name, string(hdr.Typeflag))
		}
		if rule := ValidatePath(name); rule != "" {
			return nil, fmt.Errorf("%w: %q (%s)", ErrUnsafeEntry, hdr.Name, rule)
		}
		if hdr.Size < 0 || hdr.Size > lim.MaxFileSize {
			return nil, fmt.Errorf("%w: %s is %d bytes (max %d)", ErrLimitExceeded, name, hdr.Size, lim.MaxFileSize)
		}
		total += hdr.Size
		if total > lim.MaxBundleSize {
			return nil, fmt.Errorf("%w: total size over %d bytes", ErrLimitExceeded, lim.MaxBundleSize)
		}
		content, err := io.ReadAll(io.LimitReader(tr, hdr.Size+1))
		if err != nil {
			return nil, fmt.Errorf("read bundle entry %s: %w", name, err)
		}
		if int64(len(content)) != hdr.Size {
			return nil, fmt.Errorf("%w: %s size does not match its header", ErrUnsafeEntry, name)
		}
		mode := NormalizeMode(int(hdr.Mode))
		if err := writeFileExclusive(dest, name, content, mode); err != nil {
			return nil, err
		}
		files = append(files, File{Path: name, Content: content, Mode: mode})
	}

	got, err := Hash(files)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsafeEntry, err)
	}
	if got != expectedHash {
		return nil, fmt.Errorf("%w: unpacked bundle hashes to %s, expected %s", ErrIntegrity, got, expectedHash)
	}
	return files, nil
}

// UnpackBytes is Unpack over an in-memory archive.
func UnpackBytes(archive []byte, dest, expectedHash string, lim Limits) ([]File, error) {
	return Unpack(bytes.NewReader(archive), dest, expectedHash, lim)
}

// mkdirAllNoFollow creates dest/rel one component at a time. An existing
// component must be a real directory (Lstat, so a symlink is refused).
func mkdirAllNoFollow(dest, rel string) error {
	cur := dest
	for _, seg := range strings.Split(rel, "/") {
		cur = filepath.Join(cur, seg)
		fi, err := os.Lstat(cur)
		switch {
		case err == nil:
			if !fi.IsDir() {
				return fmt.Errorf("%w: %s exists and is not a directory", ErrUnsafeEntry, rel)
			}
		case errors.Is(err, os.ErrNotExist):
			if err := os.Mkdir(cur, 0o755); err != nil {
				return fmt.Errorf("%w: mkdir %s: %v", ErrUnsafeEntry, rel, err)
			}
		default:
			return fmt.Errorf("unpack %s: %w", rel, err)
		}
	}
	return nil
}

// writeFileExclusive creates dest/rel (never existing before, never through a
// symlink) with content and mode.
func writeFileExclusive(dest, rel string, content []byte, mode int) error {
	if dir := filepath.Dir(filepath.FromSlash(rel)); dir != "." {
		if err := mkdirAllNoFollow(dest, filepath.ToSlash(dir)); err != nil {
			return err
		}
	}
	target := filepath.Join(dest, filepath.FromSlash(rel))
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, os.FileMode(mode))
	if err != nil {
		return fmt.Errorf("%w: create %s: %v", ErrUnsafeEntry, rel, err)
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		return fmt.Errorf("write %s: %w", rel, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write %s: %w", rel, err)
	}
	// O_CREAT applies the umask; set the normalized mode explicitly (no
	// symlink can be here: the file was just created with O_EXCL|O_NOFOLLOW).
	return os.Chmod(target, os.FileMode(mode))
}
