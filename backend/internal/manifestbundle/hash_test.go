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
	a := []File{{"main.tf", []byte("a")}, {"modules/x/v.tf", []byte("b")}, {"README.md", nil}}
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
	base, _ := Hash([]File{{"a.tf", []byte("x")}})
	for name, files := range map[string][]File{
		"content":          {{"a.tf", []byte("y")}},
		"path":             {{"b.tf", []byte("x")}},
		"extra file":       {{"a.tf", []byte("x")}, {"b.tf", nil}},
		"boundary shift":   {{"a.tf", []byte("x\x00b.tf\x000\x00")}},
		"empty vs missing": {},
	} {
		if h, _ := Hash(files); h == base {
			t.Fatalf("%s: collided with base", name)
		}
	}
	// moving bytes between two files' boundaries must change the hash
	h1, _ := Hash([]File{{"a", []byte("12")}, {"b", []byte("3")}})
	h2, _ := Hash([]File{{"a", []byte("1")}, {"b", []byte("23")}})
	if h1 == h2 {
		t.Fatal("content boundary is ambiguous")
	}
}

func TestHashRejectsDuplicateAndEmptyPaths(t *testing.T) {
	if _, err := Hash([]File{{"a.tf", nil}, {"a.tf", []byte("x")}}); err == nil {
		t.Fatal("duplicate path must be rejected")
	}
	if _, err := Hash([]File{{"", nil}}); err == nil {
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
		{"main.tf", []byte("resource \"null_resource\" \"x\" {}\n")},
		{"Z/bin.dat", []byte{0x00, 0xff, 0x00}},
		{"a/ü.tf", []byte("")},
	}
}

func TestVersionHashReadsVersionSnapshotOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE manifest_files (id INTEGER PRIMARY KEY, manifest_id TEXT, version_id TEXT, owner_user_id TEXT, path TEXT, content BLOB)`).Error; err != nil {
		t.Fatal(err)
	}
	for _, f := range goldenFiles() {
		db.Exec(`INSERT INTO manifest_files (manifest_id, version_id, path, content) VALUES ('mf-1', 'mfv-1', ?, ?)`, f.Path, f.Content)
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
