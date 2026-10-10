package migration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// TestManifestGitSourceImmutableRejectsDirectUpdate applies the trigger on a
// throwaway database and proves a direct UPDATE of git source fields fails.
func TestManifestGitSourceImmutableRejectsDirectUpdate(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") == "1" {
		t.Skip("SKIP_INTEGRATION=1")
	}
	adminDSN := os.Getenv("TEST_ADMIN_DSN")
	if adminDSN == "" {
		adminDSN = "host=localhost user=postgres password=postgres123 port=5432 sslmode=disable dbname=postgres"
	}
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		t.Skipf("no postgres: %v", err)
	}
	if err := admin.Ping(); err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}
	const dbName = "iac_git_immutable_test"
	admin.Exec("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()", dbName)
	admin.Exec("DROP DATABASE IF EXISTS " + dbName)
	if _, err := admin.Exec("CREATE DATABASE " + dbName); err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer func() {
		admin.Exec("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()", dbName)
		admin.Exec("DROP DATABASE IF EXISTS " + dbName)
		admin.Close()
	}()

	dsn := fmt.Sprintf("host=localhost user=postgres password=postgres123 port=5432 sslmode=disable dbname=%s", dbName)
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	// Minimal tables the trigger needs.
	for _, stmt := range []string{
		`CREATE TABLE public.organizations (id integer PRIMARY KEY)`,
		`INSERT INTO public.organizations (id) VALUES (1)`,
		`CREATE TABLE public.users (user_id varchar(20) PRIMARY KEY)`,
		`INSERT INTO public.users (user_id) VALUES ('u1')`,
		`CREATE TABLE public.manifests (
			id varchar(36) PRIMARY KEY,
			organization_id integer NOT NULL REFERENCES public.organizations(id),
			name varchar(255) NOT NULL,
			status varchar(20) DEFAULT 'draft',
			source_type varchar(16) NOT NULL DEFAULT 'native',
			git_repo_url varchar(1024),
			git_subpath varchar(512),
			github_installation_id bigint,
			created_by varchar(20) NOT NULL REFERENCES public.users(user_id),
			created_at timestamptz DEFAULT now(),
			updated_at timestamptz DEFAULT now()
		)`,
	} {
		if err := gdb.Exec(stmt).Error; err != nil {
			t.Fatalf("setup: %v\n%s", err, stmt)
		}
	}
	if err := applyManifestGitSourceImmutable(context.Background(), gdb); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := gdb.Exec(`INSERT INTO public.manifests (id, organization_id, name, source_type, git_repo_url, github_installation_id, created_by)
		VALUES ('mf-1', 1, 'm', 'git', 'https://github.com/o/r', 42, 'u1')`).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	// Allowed: update non-source fields
	if err := gdb.Exec(`UPDATE public.manifests SET name = 'm2' WHERE id = 'mf-1'`).Error; err != nil {
		t.Fatalf("name update must succeed: %v", err)
	}
	// Forbidden: change git_repo_url
	err = gdb.Exec(`UPDATE public.manifests SET git_repo_url = 'https://github.com/o/other' WHERE id = 'mf-1'`).Error
	if err == nil || !strings.Contains(err.Error(), "git_source_immutable") {
		t.Fatalf("git_repo_url update must fail with git_source_immutable, got %v", err)
	}
	err = gdb.Exec(`UPDATE public.manifests SET source_type = 'native', git_repo_url = NULL, github_installation_id = NULL WHERE id = 'mf-1'`).Error
	if err == nil || !strings.Contains(err.Error(), "git_source_immutable") {
		t.Fatalf("source_type update must fail: %v", err)
	}
}
