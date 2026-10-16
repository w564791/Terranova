package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// openVarsetOrgPG creates a throwaway PostgreSQL database with the minimal
// tables the varset org_id migrations touch. Skips when PG is unreachable.
func openVarsetOrgPG(t *testing.T) *gorm.DB {
	t.Helper()
	if os.Getenv("SKIP_INTEGRATION") == "1" {
		t.Skip("SKIP_INTEGRATION=1")
	}
	base := os.Getenv("TEST_PG_BASE_DSN")
	if base == "" {
		base = "host=localhost user=postgres password=postgres123 port=5432 sslmode=disable"
	}
	admin, err := sql.Open("pgx", base+" dbname=postgres")
	if err != nil {
		t.Skipf("no postgres: %v", err)
	}
	if err := admin.Ping(); err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}
	const dbName = "iac_varset_org_test"
	drop := func() {
		admin.Exec("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()", dbName)
		admin.Exec("DROP DATABASE IF EXISTS " + dbName)
	}
	drop()
	if _, err := admin.Exec("CREATE DATABASE " + dbName); err != nil {
		t.Fatalf("create db: %v", err)
	}
	gdb, err := gorm.Open(postgres.Open(fmt.Sprintf("%s dbname=%s", base, dbName)), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	t.Cleanup(func() {
		if s, err := gdb.DB(); err == nil {
			s.Close()
		}
		drop()
		admin.Close()
	})
	for _, s := range []string{
		`CREATE TABLE projects (id integer PRIMARY KEY, org_id integer NOT NULL)`,
		`CREATE TABLE workspace_project_relations (workspace_id varchar(50), project_id integer)`,
		`CREATE TABLE variable_sets (
			id SERIAL PRIMARY KEY,
			varset_id varchar(30) NOT NULL UNIQUE,
			name varchar(100) NOT NULL,
			description text,
			scope varchar(20) NOT NULL DEFAULT 'specific',
			is_deleted boolean NOT NULL DEFAULT false,
			created_at timestamp DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamp DEFAULT CURRENT_TIMESTAMP,
			created_by varchar(20))`,
		`CREATE UNIQUE INDEX idx_variable_sets_name ON variable_sets (name) WHERE is_deleted = false`,
		`CREATE TABLE varset_assignments (
			id SERIAL PRIMARY KEY,
			varset_id varchar(30) NOT NULL REFERENCES variable_sets(varset_id) ON DELETE CASCADE,
			scope_type varchar(20) NOT NULL,
			project_id integer REFERENCES projects(id),
			workspace_id varchar(50))`,
		`CREATE TABLE user_organizations (user_id varchar(20), org_id integer, joined_at timestamp)`,
		`CREATE TABLE permission_definitions (id varchar(50) PRIMARY KEY, resource_type varchar(50), scope_level varchar(20))`,
		`CREATE TABLE iam_role_policies (role_id integer, permission_id varchar(50), permission_level varchar(20), scope_type varchar(20))`,
		`CREATE TABLE iam_user_roles (user_id varchar(20), role_id integer, scope_type varchar(20), scope_id integer, expires_at timestamp)`,
		`INSERT INTO projects VALUES (10, 1), (20, 1), (30, 2)`,
		`INSERT INTO workspace_project_relations VALUES ('ws-a', 10), ('ws-b', 20), ('ws-other', 30)`,
		`INSERT INTO permission_definitions VALUES ('pd-vs', 'VARIABLE_SETS', 'ORGANIZATION')`,
		`INSERT INTO iam_role_policies VALUES (1, 'pd-vs', 'WRITE', 'ORGANIZATION')`,
	} {
		if err := gdb.Exec(s).Error; err != nil {
			t.Fatalf("%v\n%s", err, s)
		}
	}
	return gdb
}

func pgOrg(t *testing.T, db *gorm.DB, id string) *int64 {
	t.Helper()
	var org *int64
	if err := db.Raw(`SELECT org_id FROM variable_sets WHERE varset_id = ?`, id).Scan(&org).Error; err != nil {
		t.Fatal(err)
	}
	return org
}

func isCheckViolation(err error, constraint string) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23514" && pe.ConstraintName == constraint
}

// The four backfill cases on real PostgreSQL, then the NOT VALID check.
func TestVariableSetOrgID_PG(t *testing.T) {
	db := openVarsetOrgPG(t)
	ctx := context.Background()
	for _, s := range []string{
		// 1. single-org mounts
		`INSERT INTO variable_sets (varset_id, name, scope, created_by) VALUES ('vs-single', 'single', 'specific', 'u1')`,
		`INSERT INTO varset_assignments (varset_id, scope_type, workspace_id) VALUES ('vs-single', 'workspace', 'ws-a')`,
		`INSERT INTO varset_assignments (varset_id, scope_type, project_id) VALUES ('vs-single', 'project', 20)`,
		// 2. cross-org mounts: 2 in org 1, 1 in org 2
		`INSERT INTO variable_sets (varset_id, name, scope, created_by) VALUES ('vs-cross', 'cross', 'specific', 'u1')`,
		`INSERT INTO varset_assignments (varset_id, scope_type, workspace_id) VALUES ('vs-cross', 'workspace', 'ws-a'), ('vs-cross', 'workspace', 'ws-other')`,
		`INSERT INTO varset_assignments (varset_id, scope_type, project_id) VALUES ('vs-cross', 'project', 20)`,
		// 3a. unassigned, creator has WRITE in org 2 (joined earlier) and org 1
		`INSERT INTO variable_sets (varset_id, name, scope, created_by) VALUES ('vs-unassigned', 'unassigned', 'specific', 'u1')`,
		`INSERT INTO user_organizations VALUES ('u1', 1, '2024-02-01'), ('u1', 2, '2024-01-01')`,
		`INSERT INTO iam_user_roles VALUES ('u1', 1, 'ORGANIZATION', 1, NULL), ('u1', 1, 'ORGANIZATION', 2, NULL)`,
		// 3b. unassigned, creator without WRITE anywhere -> stays NULL (orphan)
		`INSERT INTO variable_sets (varset_id, name, scope, created_by) VALUES ('vs-orphan', 'orphan', 'specific', 'u9')`,
		`INSERT INTO user_organizations VALUES ('u9', 1, '2024-01-01')`,
		// 4. global
		`INSERT INTO variable_sets (varset_id, name, scope, created_by) VALUES ('vs-global', 'global', 'global', 'u1')`,
	} {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("%v\n%s", err, s)
		}
	}

	for i := 0; i < 2; i++ { // second run proves idempotence
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := applyVariableSetOrgID(ctx, tx); err != nil {
				return err
			}
			return applyVariableSetOrgRequired(ctx, tx)
		}); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}

	want := map[string]int64{"vs-single": 1, "vs-cross": 1, "vs-unassigned": 2}
	for id, org := range want {
		if got := pgOrg(t, db, id); got == nil || *got != org {
			t.Errorf("%s org_id = %v, want %d", id, got, org)
		}
	}
	for _, id := range []string{"vs-orphan", "vs-global"} {
		if got := pgOrg(t, db, id); got != nil {
			t.Errorf("%s org_id = %d, want NULL", id, *got)
		}
	}
	var foreign int64
	db.Raw(`SELECT COUNT(*) FROM varset_assignments a
		JOIN workspace_project_relations wpr ON wpr.workspace_id = a.workspace_id
		JOIN projects p ON p.id = wpr.project_id
		WHERE a.varset_id = 'vs-cross' AND p.org_id = 2`).Scan(&foreign)
	var kept int64
	db.Raw(`SELECT COUNT(*) FROM varset_assignments WHERE varset_id = 'vs-cross'`).Scan(&kept)
	if foreign != 0 || kept != 2 {
		t.Errorf("vs-cross mounts: org2=%d total=%d, want 0/2", foreign, kept)
	}

	// Constraint present and NOT VALID.
	var validated bool
	db.Raw(`SELECT convalidated FROM pg_constraint WHERE conname = 'chk_variable_sets_specific_org'`).Scan(&validated)
	if validated {
		t.Fatal("chk_variable_sets_specific_org must be NOT VALID while orphans exist")
	}

	// A plain UPDATE of an orphan without org_id fails the CHECK.
	err := db.Exec(`UPDATE variable_sets SET name = 'renamed' WHERE varset_id = 'vs-orphan'`).Error
	if !isCheckViolation(err, "chk_variable_sets_specific_org") {
		t.Fatalf("plain orphan UPDATE err = %v, want check violation", err)
	}
	// New specific rows need org_id.
	err = db.Exec(`INSERT INTO variable_sets (varset_id, name, scope) VALUES ('vs-new', 'new', 'specific')`).Error
	if !isCheckViolation(err, "chk_variable_sets_specific_org") {
		t.Fatalf("insert specific without org_id err = %v, want check violation", err)
	}
	// global with org_id still rejected by the 20261016_01 check.
	err = db.Exec(`UPDATE variable_sets SET org_id = 1 WHERE varset_id = 'vs-global'`).Error
	if !isCheckViolation(err, "chk_variable_sets_org_scope") {
		t.Fatalf("global with org_id err = %v, want chk_variable_sets_org_scope", err)
	}

	// VALIDATE fails while an orphan remains, succeeds once healed.
	if err := db.Exec(`ALTER TABLE variable_sets VALIDATE CONSTRAINT chk_variable_sets_specific_org`).Error; !isCheckViolation(err, "chk_variable_sets_specific_org") {
		t.Fatalf("VALIDATE with orphan err = %v, want check violation", err)
	}
	// Heal in the same UPDATE as the edit.
	if err := db.Exec(`UPDATE variable_sets SET name = 'renamed', org_id = 1 WHERE varset_id = 'vs-orphan' AND org_id IS NULL AND created_by = 'u9'`).Error; err != nil {
		t.Fatalf("heal update: %v", err)
	}
	var orphans int64
	db.Raw(`SELECT COUNT(*) FROM variable_sets WHERE scope = 'specific' AND org_id IS NULL`).Scan(&orphans)
	if orphans != 0 {
		t.Fatalf("orphans = %d after heal", orphans)
	}
	if err := db.Exec(`ALTER TABLE variable_sets VALIDATE CONSTRAINT chk_variable_sets_specific_org`).Error; err != nil {
		t.Fatalf("VALIDATE after heal: %v", err)
	}
}
