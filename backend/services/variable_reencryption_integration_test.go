package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"testing"

	"iac-platform/internal/crypto"
	"iac-platform/internal/models"

	"gorm.io/gorm"
)

func reencTestKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

type reencRow struct {
	ID         uint
	Value      string
	KeyVersion int16
}

func reencLoad(t *testing.T, db *gorm.DB, table string, id uint) reencRow {
	t.Helper()
	var r reencRow
	if err := db.Raw(fmt.Sprintf(`SELECT id, value, key_version FROM %s WHERE id = ?`, table), id).Scan(&r).Error; err != nil {
		t.Fatal(err)
	}
	return r
}

// PG: legacy (key_version 0) variable rows are re-encrypted with
// DATA_ENCRYPTION_KEY; a second (and concurrent) pass changes nothing; the
// values decrypt through the model hooks independently of JWT_SECRET; the
// startup check requires JWT_SECRET only while legacy rows remain.
func TestReencryptLegacyVariables_PG(t *testing.T) {
	db := setupVarsetTestDB(t)
	ctx := context.Background()
	for _, stmt := range []string{
		"ALTER TABLE workspace_variables ADD COLUMN IF NOT EXISTS key_version smallint NOT NULL DEFAULT 0",
		"ALTER TABLE varset_variables ADD COLUMN IF NOT EXISTS key_version smallint NOT NULL DEFAULT 0",
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	// start from a database without other legacy rows
	db.Exec(`DELETE FROM workspace_variables WHERE sensitive AND key_version = 0`)
	db.Exec(`DELETE FROM varset_variables WHERE sensitive AND key_version = 0`)

	t.Setenv("JWT_SECRET", "reenc-legacy-jwt-secret")
	t.Setenv("ENV", "development")
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	legacyCT, _, err := crypto.EncryptValueVersioned("legacy-plain")
	if err != nil || strings.HasPrefix(legacyCT, "tnk") {
		t.Fatalf("legacy encrypt: %q %v", legacyCT, err)
	}

	t.Setenv("ENV", "production")
	t.Setenv("DATA_ENCRYPTION_KEY", reencTestKey(t))
	t.Setenv("DATA_ENCRYPTION_KEY_VERSION", "")
	v1CT, _, err := crypto.EncryptValueVersioned("already-v1")
	if err != nil {
		t.Fatal(err)
	}

	ws := "ws-reenc-" + strings.ToLower(reencTestKey(t)[:8])
	ws = strings.NewReplacer("/", "x", "+", "y").Replace(ws)
	ins := func(n int, value string, sensitive bool, kv int16) uint {
		var id uint
		if err := db.Raw(`INSERT INTO workspace_variables (variable_id, workspace_id, key, version, value, sensitive, key_version)
			VALUES (?, ?, ?, 1, ?, ?, ?) RETURNING id`,
			fmt.Sprintf("var-re%s%d", ws[len(ws)-6:], n), ws, fmt.Sprintf("k%d", n), value, sensitive, kv).Scan(&id).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM workspace_variables WHERE workspace_id = ?`, ws) })
	idLegacy := ins(1, legacyCT, true, 0)
	idPlain := ins(2, "plain text secret!", true, 0) // sensitive but never encrypted
	idAmbiguous := ins(3, base64.StdEncoding.EncodeToString([]byte("not-a-ciphertext-xyz")), true, 0)
	idNonSensitive := ins(4, "public", false, 0)
	idV1 := ins(5, v1CT, true, 1)
	idRelabel := ins(6, v1CT, true, 0) // versioned value, column not set

	// varset row
	vs := models.VariableSet{VarsetID: "varset-" + ws[len(ws)-8:], Name: "reenc " + ws}
	if err := db.Create(&vs).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM variable_sets WHERE varset_id = ?`, vs.VarsetID) })
	var idVarset uint
	if err := db.Raw(`INSERT INTO varset_variables (variable_id, varset_id, key, value, sensitive, key_version)
		VALUES (?, ?, 'vk', ?, true, 0) RETURNING id`, "var-vs"+ws[len(ws)-6:], vs.VarsetID, legacyCT).Scan(&idVarset).Error; err != nil {
		t.Fatal(err)
	}

	// startup check: legacy rows present -> JWT_SECRET required
	t.Setenv("JWT_SECRET", "")
	if err := CheckLegacyKeyAvailable(ctx, db); err == nil || !strings.Contains(err.Error(), "workspace_variables=") {
		t.Fatalf("startup check with legacy rows and no JWT_SECRET: %v", err)
	}
	t.Setenv("JWT_SECRET", "reenc-legacy-jwt-secret")
	if err := CheckLegacyKeyAvailable(ctx, db); err != nil {
		t.Fatal(err)
	}

	res, err := ReencryptLegacyVariables(ctx, db, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Reencrypted != 2 || res.Encrypted != 1 || res.Relabelled != 1 || res.Undecryptable != 1 {
		t.Fatalf("first pass: %+v", res)
	}
	after := map[uint]reencRow{}
	for _, id := range []uint{idLegacy, idPlain, idAmbiguous, idNonSensitive, idV1, idRelabel} {
		after[id] = reencLoad(t, db, "workspace_variables", id)
	}
	if r := after[idLegacy]; r.KeyVersion != 1 || !strings.HasPrefix(r.Value, "tnk1:") {
		t.Fatalf("legacy row not re-encrypted: %+v", r)
	}
	if r := after[idPlain]; r.KeyVersion != 1 || !strings.HasPrefix(r.Value, "tnk1:") {
		t.Fatalf("plaintext sensitive row not encrypted: %+v", r)
	}
	if r := after[idAmbiguous]; r.KeyVersion != 0 {
		t.Fatalf("ambiguous row touched: %+v", r)
	}
	if r := after[idNonSensitive]; r.KeyVersion != 0 || r.Value != "public" {
		t.Fatalf("non-sensitive row touched: %+v", r)
	}
	if r := after[idV1]; r.Value != v1CT || r.KeyVersion != 1 {
		t.Fatalf("v1 row touched: %+v", r)
	}
	if r := after[idRelabel]; r.Value != v1CT || r.KeyVersion != 1 {
		t.Fatalf("relabel: %+v", r)
	}
	if r := reencLoad(t, db, "varset_variables", idVarset); r.KeyVersion != 1 || !strings.HasPrefix(r.Value, "tnk1:") {
		t.Fatalf("varset row not re-encrypted: %+v", r)
	}

	// idempotent: second pass (and two concurrent passes) change nothing
	var wg sync.WaitGroup
	results := make([]VariableReencryptionResult, 3)
	errs := make([]error, 3)
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = ReencryptLegacyVariables(ctx, db, 2) }(i)
	}
	wg.Wait()
	for i := range results {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if results[i].Reencrypted+results[i].Encrypted+results[i].Relabelled != 0 {
			t.Fatalf("pass %d changed rows: %+v", i, results[i])
		}
	}
	for id, before := range after {
		if r := reencLoad(t, db, "workspace_variables", id); r != before {
			t.Fatalf("row %d changed on re-run: %+v -> %+v", id, before, r)
		}
	}

	// decrypts through the model hooks, JWT_SECRET irrelevant now
	t.Setenv("JWT_SECRET", "something-else-entirely")
	var v models.WorkspaceVariable
	if err := db.First(&v, idLegacy).Error; err != nil {
		t.Fatal(err)
	}
	if v.Value != "legacy-plain" || v.KeyVersion != 1 {
		t.Fatalf("model read: %q v%d", v.Value, v.KeyVersion)
	}
	var vv models.VarsetVariable
	if err := db.First(&vv, idVarset).Error; err != nil || vv.Value != "legacy-plain" {
		t.Fatalf("varset model read: %q %v", vv.Value, err)
	}

	// only the ambiguous row is still legacy
	counts, err := CountLegacyVariableRows(ctx, db)
	if err != nil || counts["workspace_variables"] != 1 || counts["varset_variables"] != 0 {
		t.Fatalf("remaining legacy rows: %v %v", counts, err)
	}
	db.Exec(`DELETE FROM workspace_variables WHERE id = ?`, idAmbiguous)
	t.Setenv("JWT_SECRET", "")
	if err := CheckLegacyKeyAvailable(ctx, db); err != nil {
		t.Fatalf("no legacy rows left, JWT_SECRET unset: %v", err)
	}

	// new writes via the model carry key_version
	nv := models.WorkspaceVariable{WorkspaceID: ws, Key: "k-new", Value: "fresh", Sensitive: true, Version: 1}
	if err := db.Create(&nv).Error; err != nil {
		t.Fatal(err)
	}
	if r := reencLoad(t, db, "workspace_variables", nv.ID); r.KeyVersion != 1 || !strings.HasPrefix(r.Value, "tnk1:") {
		t.Fatalf("new model write: %+v", r)
	}
}
