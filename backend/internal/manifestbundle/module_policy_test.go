package manifestbundle

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPublishModuleSourcePolicy_GitPinning(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE modules (id INTEGER PRIMARY KEY, status TEXT, module_source TEXT)`,
		`CREATE TABLE module_versions (id INTEGER PRIMARY KEY, module_id INTEGER, module_source TEXT)`,
		`INSERT INTO modules (id, status, module_source) VALUES
			(1, 'active', 'terraform-aws-modules/vpc/aws'),
			(2, 'active', 'git::https://github.com/acme/net.git?ref=v1.2.0'),
			(3, 'active', 'git::https://github.com/acme/dns.git'),
			(4, 'inactive', 'git::https://github.com/acme/old.git')`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	sha := "0123456789abcdef0123456789abcdef01234567"
	policy := PublishModuleSourcePolicy(context.Background(), db)
	for source, want := range map[string]bool{
		"terraform-aws-modules/vpc/aws":                                 true,
		"git::https://github.com/acme/net.git?ref=" + sha:               true, // catalog lists it at a tag; the bundle pins
		"git::https://github.com/acme/net.git//sub?ref=" + sha:          true,
		"git::https://github.com/acme/dns.git?ref=" + sha:               true,
		"git::https://github.com/acme/net.git?ref=v1.2.0":               false, // exact catalog entry, but a tag
		"git::https://github.com/acme/dns.git":                          false, // unpinned
		"git::https://github.com/acme/old.git?ref=" + sha:               false, // inactive module
		"git::https://github.com/evil/net.git?ref=" + sha:               false, // not in the catalog
		"git::https://github.com/acme/net.git?ref=" + sha + "&sshkey=x": false,
	} {
		got, err := policy(source)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("policy(%q) = %v, want %v", source, got, want)
		}
	}
}
