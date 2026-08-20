package service

import (
	"context"
	"testing"
	"time"

	"iac-platform/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupPoolTokenServiceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`
CREATE TABLE agent_pools (
  pool_id TEXT PRIMARY KEY,
  name TEXT,
  pool_type TEXT,
  k8s_config TEXT,
  created_at DATETIME,
  updated_at DATETIME,
  updated_by TEXT
);
CREATE TABLE pool_tokens (
  token_hash TEXT PRIMARY KEY,
  token_name TEXT,
  token_type TEXT,
  pool_id TEXT,
  is_active INTEGER,
  created_at DATETIME,
  created_by TEXT,
  revoked_at DATETIME,
  revoked_by TEXT,
  last_used_at DATETIME,
  expires_at DATETIME,
  k8s_job_name TEXT,
  k8s_pod_name TEXT,
  k8s_namespace TEXT DEFAULT 'terraform'
);`)
	_ = db.Exec(`INSERT INTO agent_pools (pool_id, name, pool_type) VALUES ('pool-static','s','static')`)
	_ = db.Exec(`INSERT INTO agent_pools (pool_id, name, pool_type) VALUES ('pool-k8s','k','k8s')`)
	return db
}

func TestPoolTokenService_GenerateListRevokeValidate(t *testing.T) {
	db := setupPoolTokenServiceDB(t)
	svc := NewPoolTokenService(db)
	ctx := context.Background()

	resp, err := svc.GenerateStaticToken(ctx, "pool-static", "tok-a", "user-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Token == "" || resp.PoolID != "pool-static" {
		t.Fatalf("%+v", resp)
	}

	// unknown pool
	if _, err := svc.GenerateStaticToken(ctx, "nope", "x", "u", nil); err == nil {
		t.Fatal("unknown pool")
	}

	// k8s temporary on static pool fails
	if _, err := svc.GenerateK8sTemporaryToken(ctx, "pool-static", "job", "pod", "u", time.Now().Add(time.Hour)); err == nil {
		t.Fatal("k8s token on static pool")
	}

	k8s, err := svc.GenerateK8sTemporaryToken(ctx, "pool-k8s", "job1", "pod1", "u", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if k8s.TokenType != models.PoolTokenTypeK8sTemporary {
		t.Fatal(k8s.TokenType)
	}

	list, err := svc.ListPoolTokens(ctx, "pool-static")
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %d", err, len(list))
	}

	// validate
	tok, err := svc.ValidateToken(ctx, resp.Token)
	if err != nil || !tok.IsActive {
		t.Fatalf("validate: %v %+v", err, tok)
	}
	if _, err := svc.ValidateToken(ctx, "apt_bad"); err == nil {
		t.Fatal("bad token")
	}

	// revoke
	if err := svc.RevokeToken(ctx, "pool-static", "tok-a", "user-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateToken(ctx, resp.Token); err == nil {
		t.Fatal("revoked must fail")
	}
	if err := svc.RevokeToken(ctx, "pool-static", "tok-a", "user-1"); err == nil {
		t.Fatal("second revoke")
	}
}

func TestPoolTokenService_ValidateExpiredAndCleanup(t *testing.T) {
	db := setupPoolTokenServiceDB(t)
	svc := NewPoolTokenService(db)
	ctx := context.Background()

	past := time.Now().Add(-time.Hour)
	resp, err := svc.GenerateStaticToken(ctx, "pool-static", "exp", "u", &past)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateToken(ctx, resp.Token); err == nil {
		t.Fatal("expired validate")
	}
	n, err := svc.CleanupExpiredTokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("cleanup rows=%d", n)
	}
}

func TestPoolTokenService_K8sConfig(t *testing.T) {
	db := setupPoolTokenServiceDB(t)
	svc := NewPoolTokenService(db)
	ctx := context.Background()

	// static pool cannot update k8s config
	if err := svc.UpdateK8sConfig(ctx, "pool-static", models.K8sJobTemplateConfig{Image: "x"}, "u"); err == nil {
		t.Fatal("static pool k8s config")
	}

	cfg := models.K8sJobTemplateConfig{
		Image: "registry/agent:1", ImagePullPolicy: "IfNotPresent", MinReplicas: 1, MaxReplicas: 5,
	}
	if err := svc.UpdateK8sConfig(ctx, "pool-k8s", cfg, "admin"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetK8sConfig(ctx, "pool-k8s")
	if err != nil {
		t.Fatal(err)
	}
	if got.Image != "registry/agent:1" {
		t.Fatalf("%+v", got)
	}

	// empty config path
	_ = db.Exec(`UPDATE agent_pools SET k8s_config = NULL WHERE pool_id = 'pool-k8s'`)
	empty, err := svc.GetK8sConfig(ctx, "pool-k8s")
	if err != nil || empty.MinReplicas != 1 {
		t.Fatalf("empty defaults: %v %+v", err, empty)
	}

	if _, err := svc.GetK8sConfig(ctx, "pool-static"); err == nil {
		t.Fatal("static get k8s")
	}
	if _, err := svc.GetK8sConfig(ctx, "missing"); err == nil {
		t.Fatal("missing pool")
	}
}
