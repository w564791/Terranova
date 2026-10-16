package services

import (
	"fmt"
	"sync"
	"testing"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

func varsetOrg(t *testing.T, svc *VariableSetService, id string) *uint {
	t.Helper()
	var vs models.VariableSet
	if err := svc.db.Where("varset_id = ?", id).First(&vs).Error; err != nil {
		t.Fatal(err)
	}
	return vs.OrgID
}

// Sequential "lost race": the first heal binds org 1; a later heal (any path)
// targeting org 2 must 404 and never overwrite. A repeat heal from the same
// creator+org (concurrent duplicate) proceeds.
func TestHealClaimSecondOrgGets404(t *testing.T) {
	db := setupVarsetOrgDB(t)
	svc := NewVariableSetService(db)
	if err := db.Exec(`INSERT INTO variable_sets (varset_id, name, scope, org_id, created_by, is_deleted)
		VALUES ('vs-o', 'orphan', 'specific', NULL, 'u1', 0)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.HealOrphan("vs-o", 1, "u1"); err != nil {
		t.Fatalf("first heal: %v", err)
	}
	c2 := &HealClaim{OrgID: 2, CallerID: "u1"}
	name := "renamed"
	ws := "ws-b"
	for label, err := range map[string]error{
		"pre-write": svc.HealOrphan("vs-o", 2, "u1"),
		"tx-claim":  db.Transaction(func(tx *gorm.DB) error { return claimOrphanTx(tx, "vs-o", *c2, map[string]interface{}{"name": name}) }),
		"mount": func() error {
			_, e := svc.CreateAssignment("vs-o", "workspace", nil, &ws, vsStrPtr("u1"), 2, c2)
			return e
		}(),
	} {
		if err == nil {
			t.Fatalf("%s heal into org 2 succeeded, want not found", label)
		}
	}
	if o := varsetOrg(t, svc, "vs-o"); o == nil || *o != 1 {
		t.Fatalf("org_id = %v, want 1 (no overwrite)", o)
	}
	// Same creator, same org: duplicate claim proceeds and applies the edit.
	if err := db.Transaction(func(tx *gorm.DB) error {
		return claimOrphanTx(tx, "vs-o", HealClaim{OrgID: 1, CallerID: "u1"}, map[string]interface{}{"name": name})
	}); err != nil {
		t.Fatalf("same-org duplicate claim: %v", err)
	}
}

// Real concurrency on PG: two heals race for different orgs; exactly one
// wins, the loser gets not found, and org_id is the winner's.
func TestHealClaimRacePG(t *testing.T) {
	if testDB == nil {
		t.Skip("no PG")
	}
	db := testDB
	db.Exec(`ALTER TABLE variable_sets DROP CONSTRAINT IF EXISTS chk_variable_sets_specific_org`)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM variable_sets WHERE varset_id LIKE 'vs-race-%'`)
		db.Exec(`ALTER TABLE variable_sets ADD CONSTRAINT chk_variable_sets_specific_org CHECK (scope <> 'specific' OR org_id IS NOT NULL) NOT VALID`)
	})
	svc := NewVariableSetService(db)
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("vs-race-%d", i)
		if err := db.Exec(`INSERT INTO variable_sets (varset_id, name, scope, org_id, created_by, is_deleted, created_at, updated_at)
			VALUES (?, ?, 'specific', NULL, 'u1', false, now(), now())`, id, id).Error; err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make([]error, 2)
		start := make(chan struct{})
		for k, org := range []uint{1, 2} {
			wg.Add(1)
			go func(k int, org uint) {
				defer wg.Done()
				<-start
				if k == 0 {
					errs[k] = svc.HealOrphan(id, org, "u1")
				} else {
					n := "x"
					_, errs[k] = svc.Update(id, nil, &n, &HealClaim{OrgID: org, CallerID: "u1"})
				}
			}(k, org)
		}
		close(start)
		wg.Wait()
		ok := 0
		var winner uint
		for k, e := range errs {
			if e == nil {
				ok++
				winner = uint(k + 1)
			}
		}
		if ok != 1 {
			t.Fatalf("%s: %d heals succeeded (%v), want exactly 1", id, ok, errs)
		}
		if o := varsetOrg(t, svc, id); o == nil || *o != winner {
			t.Fatalf("%s: org_id = %v, want winner %d", id, o, winner)
		}
	}
}
