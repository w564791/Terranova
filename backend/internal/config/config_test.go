package config

import "testing"

func TestLoadDatabaseDoesNotRequireRuntimeSecrets(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("DB_HOST", "db.example.test")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "migration_test")
	t.Setenv("DB_USER", "migration_user")
	t.Setenv("DB_PASSWORD", "migration_password")
	t.Setenv("DB_SSLMODE", "disable")

	got := LoadDatabase()
	if got.Host != "db.example.test" || got.Port != "5432" || got.Name != "migration_test" ||
		got.User != "migration_user" || got.Password != "migration_password" || got.SSLMode != "disable" {
		t.Fatalf("unexpected database config: %+v", got)
	}
}
