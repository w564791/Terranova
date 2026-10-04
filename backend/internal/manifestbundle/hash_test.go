package manifestbundle

import (
	"context"
	"regexp"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestHashIsOrderIndependentAndDeterministic(t *testing.T) {
	a := []File{{Path: "main.tf", Content: []byte("a")}, {Path: "modules/x/v.tf", Content: []byte("b")}, {Path: "README.md", Content: nil}}
	b := []File{a[2], a[0], a[1]}
	ha, err := Hash(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, _ := Hash(b)
	if ha != hb {
		t.Fatalf("order changed hash: %s vs %s", ha, hb)
	}
	if again, _ := Hash(a); again != ha {
		t.Fatal("hash not deterministic")
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(ha) {
		t.Fatalf("not lowercase hex sha256: %s", ha)
	}
	// input slice must not be reordered
	if a[0].Path != "main.tf" {
		t.Fatal("Hash mutated its input")
	}
}

func TestHashDistinguishesContentPathAndBoundaries(t *testing.T) {
	base, _ := Hash([]File{{Path: "a.tf", Content: []byte("x")}})
	for name, files := range map[string][]File{
		"content":          {{Path: "a.tf", Content: []byte("y")}},
		"path":             {{Path: "b.tf", Content: []byte("x")}},
		"extra file":       {{Path: "a.tf", Content: []byte("x")}, {Path: "b.tf", Content: nil}},
		"boundary shift":   {{Path: "a.tf", Content: []byte("x\x00b.tf\x000\x00")}},
		"empty vs missing": {},
	} {
		if h, _ := Hash(files); h == base {
			t.Fatalf("%s: collided with base", name)
		}
	}
	// moving bytes between two files' boundaries must change the hash
	h1, _ := Hash([]File{{Path: "a", Content: []byte("12")}, {Path: "b", Content: []byte("3")}})
	h2, _ := Hash([]File{{Path: "a", Content: []byte("1")}, {Path: "b", Content: []byte("23")}})
	if h1 == h2 {
		t.Fatal("content boundary is ambiguous")
	}
}

func TestHashRejectsDuplicateAndEmptyPaths(t *testing.T) {
	if _, err := Hash([]File{{Path: "a.tf", Content: nil}, {Path: "a.tf", Content: []byte("x")}}); err == nil {
		t.Fatal("duplicate path must be rejected")
	}
	if _, err := Hash([]File{{Path: "", Content: nil}}); err == nil {
		t.Fatal("empty path must be rejected")
	}
}

// Golden vectors pin the encoding. The same values were produced by the SQL
// backfill expression in backend/migrations/add_manifest_sandbox_schema.sql on
// PostgreSQL 17; changing either side must fail here.
func TestHashGoldenVector(t *testing.T) {
	empty, _ := Hash(nil)
	if empty != goldenEmpty {
		t.Fatalf("empty bundle hash = %s, want %s", empty, goldenEmpty)
	}
	h, _ := Hash(goldenFiles())
	if h != goldenBundle {
		t.Fatalf("golden bundle hash = %s, want %s", h, goldenBundle)
	}
}

func goldenFiles() []File {
	return []File{
		{Path: "main.tf", Content: []byte("resource \"null_resource\" \"x\" {}\n"), Mode: 0o644},
		{Path: "Z/bin.dat", Content: []byte{0x00, 0xff, 0x00}, Mode: 0o775}, // -> 755
		{Path: "a/ü.tf", Content: []byte(""), Mode: 0},                      // unknown -> 644
	}
}

func TestVersionHashReadsVersionSnapshotOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE manifest_files (id INTEGER PRIMARY KEY, manifest_id TEXT, version_id TEXT, owner_user_id TEXT, path TEXT, content BLOB, mode INTEGER NOT NULL DEFAULT 420)`).Error; err != nil {
		t.Fatal(err)
	}
	for _, f := range goldenFiles() {
		db.Exec(`INSERT INTO manifest_files (manifest_id, version_id, path, content, mode) VALUES ('mf-1', 'mfv-1', ?, ?, ?)`, f.Path, f.Content, f.Mode)
	}
	// draft row and another version must not affect mfv-1
	db.Exec(`INSERT INTO manifest_files (manifest_id, version_id, owner_user_id, path, content) VALUES ('mf-1', NULL, 'u1', 'main.tf', X'01')`)
	db.Exec(`INSERT INTO manifest_files (manifest_id, version_id, path, content) VALUES ('mf-1', 'mfv-2', 'other.tf', X'01')`)

	got, err := VersionHash(context.Background(), db, "mfv-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != goldenBundle {
		t.Fatalf("VersionHash = %s, want %s", got, goldenBundle)
	}
	if none, _ := VersionHash(context.Background(), db, "mfv-none"); none != goldenEmpty {
		t.Fatalf("version without files = %s, want empty-bundle hash", none)
	}
}

func TestHashIncludesNormalizedMode(t *testing.T) {
	h := func(mode int) string {
		v, _ := Hash([]File{{Path: "run.sh", Content: []byte("echo hi"), Mode: mode}})
		return v
	}
	if h(0o644) == h(0o755) {
		t.Fatal("a mode change must alter the hash")
	}
	// only the executable bit matters
	for _, m := range []int{0, 0o600, 0o644, 0o664, 0o666, 0o100644} {
		if h(m) != h(0o644) {
			t.Fatalf("mode %o must hash as 644", m)
		}
	}
	for _, m := range []int{0o700, 0o744, 0o755, 0o775, 0o100, 0o001, 0o100755} {
		if h(m) != h(0o755) {
			t.Fatalf("mode %o must hash as 755", m)
		}
	}
}

func TestModeFromGit(t *testing.T) {
	for in, want := range map[string]int{"100644": ModeRegular, "100664": ModeRegular, "100755": ModeExecutable} {
		if got, err := ModeFromGit(in); err != nil || got != want {
			t.Fatalf("ModeFromGit(%s) = %o, %v; want %o", in, got, err, want)
		}
	}
	for _, bad := range []string{"120000", "160000", "040000", "", "abc"} {
		if _, err := ModeFromGit(bad); err == nil {
			t.Fatalf("ModeFromGit(%q) must fail", bad)
		}
	}
	// a git 100755 file hashes like a stored 0755 row
	m, _ := ModeFromGit("100755")
	a, _ := Hash([]File{{Path: "x", Mode: m}})
	b, _ := Hash([]File{{Path: "x", Mode: 493}})
	if a != b {
		t.Fatal("git and stored modes must hash the same")
	}
}
