package manifestbundle

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testBundle(t *testing.T) ([]File, string) {
	t.Helper()
	files := []File{
		{Path: "envs/prod/main.tf", Content: []byte("module \"m\" { source = \"../../modules/m\" }\n"), Mode: 0o644},
		{Path: "modules/m/main.tf", Content: []byte("output \"x\" { value = 1 }\n"), Mode: 0o644},
		{Path: "scripts/run.sh", Content: []byte("#!/bin/sh\n"), Mode: 0o755},
	}
	h, err := Hash(files)
	if err != nil {
		t.Fatal(err)
	}
	return files, h
}

type entry struct {
	hdr  tar.Header
	body []byte
}

func rawArchive(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		h := e.hdr
		if h.Typeflag == tar.TypeReg && h.Size == 0 {
			h.Size = int64(len(e.body))
		}
		if h.Mode == 0 {
			h.Mode = 0o644
		}
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if len(e.body) > 0 {
			if _, err := tw.Write(e.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestArchiveRoundTripVerifiesHash(t *testing.T) {
	files, h := testBundle(t)
	a1, err := ArchiveBytes(files)
	if err != nil {
		t.Fatal(err)
	}
	// deterministic regardless of input order
	rev := []File{files[2], files[0], files[1]}
	a2, _ := ArchiveBytes(rev)
	if !bytes.Equal(a1, a2) {
		t.Fatal("archive not deterministic")
	}
	dest := t.TempDir()
	got, err := UnpackBytes(a1, dest, h, DefaultLimits())
	if err != nil {
		t.Fatalf("unpack: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d files", len(got))
	}
	b, err := os.ReadFile(filepath.Join(dest, "modules/m/main.tf"))
	if err != nil || string(b) != string(files[1].Content) {
		t.Fatalf("content: %q %v", b, err)
	}
	fi, _ := os.Stat(filepath.Join(dest, "scripts/run.sh"))
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v", fi.Mode())
	}
	fi, _ = os.Stat(filepath.Join(dest, "modules/m/main.tf"))
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v", fi.Mode())
	}
}

func TestUnpackRejectsHashMismatchAndNullHash(t *testing.T) {
	files, _ := testBundle(t)
	a, _ := ArchiveBytes(files)

	_, err := UnpackBytes(a, t.TempDir(), strings.Repeat("0", 64), DefaultLimits())
	if !errors.Is(err, ErrIntegrity) {
		t.Fatalf("want ErrIntegrity, got %v", err)
	}

	_, err = UnpackBytes(a, t.TempDir(), "", DefaultLimits())
	var rr *RepublishRequiredError
	if !errors.As(err, &rr) || err.Error() != "bundle_republish_required: no valid bundle" {
		t.Fatalf("want RepublishRequiredError, got %v", err)
	}

	// mode is part of the hash: same bytes, flipped exec bit => mismatch
	files2 := append([]File(nil), files...)
	files2[0].Mode = 0o755
	h2, _ := Hash(files2)
	if _, err := UnpackBytes(a, t.TempDir(), h2, DefaultLimits()); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("mode change must break the hash, got %v", err)
	}
}

func TestUnpackRejectsUnsafeEntries(t *testing.T) {
	files, h := testBundle(t)
	_ = files
	cases := map[string][]entry{
		"traversal":        {{hdr: tar.Header{Typeflag: tar.TypeReg, Name: "../evil.tf"}, body: []byte("x")}},
		"nested-dotdot":    {{hdr: tar.Header{Typeflag: tar.TypeReg, Name: "a/../../evil.tf"}, body: []byte("x")}},
		"absolute":         {{hdr: tar.Header{Typeflag: tar.TypeReg, Name: "/etc/evil.tf"}, body: []byte("x")}},
		"backslash":        {{hdr: tar.Header{Typeflag: tar.TypeReg, Name: "a\\..\\evil.tf"}, body: []byte("x")}},
		"dot-segment":      {{hdr: tar.Header{Typeflag: tar.TypeReg, Name: "./main.tf"}, body: []byte("x")}},
		"control-char":     {{hdr: tar.Header{Typeflag: tar.TypeReg, Name: "ma\nin.tf"}, body: []byte("x")}},
		"too-long":         {{hdr: tar.Header{Typeflag: tar.TypeReg, Name: strings.Repeat("a", MaxPathLen+1)}, body: []byte("x")}},
		"dir-traversal":    {{hdr: tar.Header{Typeflag: tar.TypeDir, Name: "../x/", Mode: 0o755}}},
		"symlink":          {{hdr: tar.Header{Typeflag: tar.TypeSymlink, Name: "link.tf", Linkname: "/etc/passwd"}}},
		"relative-symlink": {{hdr: tar.Header{Typeflag: tar.TypeSymlink, Name: "link.tf", Linkname: "main.tf"}}},
		"hardlink":         {{hdr: tar.Header{Typeflag: tar.TypeLink, Name: "hard.tf", Linkname: "main.tf"}}},
		"char-device":      {{hdr: tar.Header{Typeflag: tar.TypeChar, Name: "null", Devmajor: 1, Devminor: 3}}},
		"block-device":     {{hdr: tar.Header{Typeflag: tar.TypeBlock, Name: "sda", Devmajor: 8}}},
		"fifo":             {{hdr: tar.Header{Typeflag: tar.TypeFifo, Name: "pipe"}}},
		"symlink-then-file-through-it": {
			{hdr: tar.Header{Typeflag: tar.TypeSymlink, Name: "d", Linkname: "/tmp"}},
			{hdr: tar.Header{Typeflag: tar.TypeReg, Name: "d/evil.tf"}, body: []byte("x")},
		},
		"duplicate": {
			{hdr: tar.Header{Typeflag: tar.TypeReg, Name: "main.tf"}, body: []byte("a")},
			{hdr: tar.Header{Typeflag: tar.TypeReg, Name: "main.tf"}, body: []byte("b")},
		},
		"file-then-dir-same-name": {
			{hdr: tar.Header{Typeflag: tar.TypeReg, Name: "a"}, body: []byte("a")},
			{hdr: tar.Header{Typeflag: tar.TypeReg, Name: "a/b.tf"}, body: []byte("b")},
		},
	}
	for name, es := range cases {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			dest := filepath.Join(parent, "work")
			if err := os.Mkdir(dest, 0o700); err != nil {
				t.Fatal(err)
			}
			_, err := UnpackBytes(rawArchive(t, es...), dest, h, DefaultLimits())
			if !errors.Is(err, ErrUnsafeEntry) {
				t.Fatalf("want ErrUnsafeEntry, got %v", err)
			}
			// nothing escaped dest
			ents, _ := os.ReadDir(parent)
			if len(ents) != 1 {
				t.Fatalf("unexpected entries next to dest: %v", ents)
			}
			if _, err := os.Lstat("/tmp/evil.tf"); err == nil {
				t.Fatal("wrote through symlink")
			}
		})
	}
}

func TestUnpackRefusesPlantedSymlinksInDest(t *testing.T) {
	files, h := testBundle(t)
	a, _ := ArchiveBytes(files)
	outside := t.TempDir()

	// a planted directory symlink on the path of an entry
	dest := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dest, "modules")); err != nil {
		t.Fatal(err)
	}
	if _, err := UnpackBytes(a, dest, h, DefaultLimits()); !errors.Is(err, ErrUnsafeEntry) {
		t.Fatalf("want ErrUnsafeEntry, got %v", err)
	}
	if ents, _ := os.ReadDir(outside); len(ents) != 0 {
		t.Fatalf("wrote through dir symlink: %v", ents)
	}

	// a planted file symlink at an entry path (O_NOFOLLOW|O_EXCL)
	dest = t.TempDir()
	target := filepath.Join(outside, "victim")
	os.WriteFile(target, []byte("keep"), 0o600)
	os.MkdirAll(filepath.Join(dest, "scripts"), 0o755)
	if err := os.Symlink(target, filepath.Join(dest, "scripts/run.sh")); err != nil {
		t.Fatal(err)
	}
	if _, err := UnpackBytes(a, dest, h, DefaultLimits()); !errors.Is(err, ErrUnsafeEntry) {
		t.Fatalf("want ErrUnsafeEntry, got %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "keep" {
		t.Fatal("victim overwritten")
	}

	// dest itself a symlink
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(outside, link)
	if _, err := UnpackBytes(a, link, h, DefaultLimits()); !errors.Is(err, ErrUnsafeEntry) {
		t.Fatalf("want ErrUnsafeEntry for symlinked dest, got %v", err)
	}
}

// countingReader fails the test when more than max bytes are pulled, proving
// Unpack stops at the offending header instead of reading the whole archive.
type countingReader struct {
	r   io.Reader
	n   int64
	max int64
	t   *testing.T
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.n > c.max {
		c.t.Fatalf("read %d bytes past the limit check (max %d)", c.n, c.max)
	}
	return n, err
}

func TestUnpackLimitsStopImmediately(t *testing.T) {
	lim := Limits{MaxFileSize: 100, MaxBundleSize: 250, MaxEntries: 3}
	small := bytes.Repeat([]byte("a"), 100)
	huge := bytes.Repeat([]byte("b"), 1<<20)

	t.Run("file-too-large", func(t *testing.T) {
		a := rawArchive(t, entry{hdr: tar.Header{Typeflag: tar.TypeReg, Name: "big.tf"}, body: huge})
		cr := &countingReader{r: bytes.NewReader(a), max: 64 << 10, t: t}
		_, err := Unpack(cr, t.TempDir(), "x", lim)
		if !errors.Is(err, ErrLimitExceeded) {
			t.Fatalf("want ErrLimitExceeded, got %v", err)
		}
	})
	t.Run("bundle-too-large", func(t *testing.T) {
		a := rawArchive(t,
			entry{hdr: tar.Header{Typeflag: tar.TypeReg, Name: "a.tf"}, body: small},
			entry{hdr: tar.Header{Typeflag: tar.TypeReg, Name: "b.tf"}, body: small},
			entry{hdr: tar.Header{Typeflag: tar.TypeReg, Name: "c.tf"}, body: small},
		)
		dest := t.TempDir()
		_, err := Unpack(bytes.NewReader(a), dest, "x", lim)
		if !errors.Is(err, ErrLimitExceeded) {
			t.Fatalf("want ErrLimitExceeded, got %v", err)
		}
		if _, err := os.Stat(filepath.Join(dest, "c.tf")); err == nil {
			t.Fatal("entry over the cumulative limit was written")
		}
	})
	t.Run("too-many-entries", func(t *testing.T) {
		var es []entry
		for _, n := range []string{"a", "b", "c", "d", "e"} {
			es = append(es, entry{hdr: tar.Header{Typeflag: tar.TypeReg, Name: n + ".tf"}, body: []byte("x")})
		}
		dest := t.TempDir()
		_, err := Unpack(bytes.NewReader(rawArchive(t, es...)), dest, "x", lim)
		if !errors.Is(err, ErrLimitExceeded) {
			t.Fatalf("want ErrLimitExceeded, got %v", err)
		}
		if _, err := os.Stat(filepath.Join(dest, "d.tf")); err == nil {
			t.Fatal("entry past MaxEntries was written")
		}
	})
	t.Run("dirs-count", func(t *testing.T) {
		var es []entry
		for _, n := range []string{"a", "b", "c", "d"} {
			es = append(es, entry{hdr: tar.Header{Typeflag: tar.TypeDir, Name: n + "/", Mode: 0o755}})
		}
		_, err := Unpack(bytes.NewReader(rawArchive(t, es...)), t.TempDir(), "x", lim)
		if !errors.Is(err, ErrLimitExceeded) {
			t.Fatalf("want ErrLimitExceeded, got %v", err)
		}
	})
}

func TestUnpackAcceptsDirectoryEntries(t *testing.T) {
	f := File{Path: "a/b/main.tf", Content: []byte("x"), Mode: 0o644}
	h, _ := Hash([]File{f})
	a := rawArchive(t,
		entry{hdr: tar.Header{Typeflag: tar.TypeDir, Name: "a/", Mode: 0o755}},
		entry{hdr: tar.Header{Typeflag: tar.TypeDir, Name: "a/b/", Mode: 0o755}},
		entry{hdr: tar.Header{Typeflag: tar.TypeReg, Name: "a/b/main.tf"}, body: []byte("x")},
	)
	if _, err := UnpackBytes(a, t.TempDir(), h, DefaultLimits()); err != nil {
		t.Fatal(err)
	}
}
