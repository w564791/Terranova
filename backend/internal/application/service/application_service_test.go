package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"iac-platform/internal/domain/entity"
)

type mockAppRepo struct {
	apps       map[uint]*entity.Application
	byKey      map[string]*entity.Application
	nextID     uint
	lastUsed   []uint
	regenCalls []struct {
		id     uint
		secret string
	}
	createErr error
	getErr    error
}

func newMockAppRepo() *mockAppRepo {
	return &mockAppRepo{
		apps:  make(map[uint]*entity.Application),
		byKey: make(map[string]*entity.Application),
	}
}

func (m *mockAppRepo) Create(ctx context.Context, app *entity.Application) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.nextID++
	app.ID = m.nextID
	app.CreatedAt = time.Now()
	cp := *app
	m.apps[app.ID] = &cp
	m.byKey[app.AppKey] = &cp
	return nil
}

func (m *mockAppRepo) GetByID(ctx context.Context, id uint) (*entity.Application, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	a, ok := m.apps[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *a
	return &cp, nil
}

func (m *mockAppRepo) GetByAppKey(ctx context.Context, appKey string) (*entity.Application, error) {
	a, ok := m.byKey[appKey]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *a
	return &cp, nil
}

func (m *mockAppRepo) ListByOrg(ctx context.Context, orgID uint, isActive *bool) ([]*entity.Application, error) {
	var out []*entity.Application
	for _, a := range m.apps {
		if a.OrgID != orgID {
			continue
		}
		if isActive != nil && a.IsActive != *isActive {
			continue
		}
		cp := *a
		out = append(out, &cp)
	}
	return out, nil
}

func (m *mockAppRepo) Update(ctx context.Context, app *entity.Application) error {
	if _, ok := m.apps[app.ID]; !ok {
		return errors.New("not found")
	}
	cp := *app
	m.apps[app.ID] = &cp
	m.byKey[app.AppKey] = &cp
	return nil
}

func (m *mockAppRepo) Delete(ctx context.Context, id uint) error {
	a, ok := m.apps[id]
	if !ok {
		return errors.New("not found")
	}
	delete(m.byKey, a.AppKey)
	delete(m.apps, id)
	return nil
}

func (m *mockAppRepo) UpdateLastUsed(ctx context.Context, id uint) error {
	m.lastUsed = append(m.lastUsed, id)
	return nil
}

func (m *mockAppRepo) RegenerateSecret(ctx context.Context, id uint, newSecret string) error {
	m.regenCalls = append(m.regenCalls, struct {
		id     uint
		secret string
	}{id, newSecret})
	a, ok := m.apps[id]
	if !ok {
		return errors.New("not found")
	}
	a.AppSecret = newSecret
	return nil
}

func TestApplicationService_CreateAndValidate(t *testing.T) {
	repo := newMockAppRepo()
	svc := NewApplicationService(repo)

	app, plainSecret, err := svc.CreateApplication(context.Background(), &CreateApplicationRequest{
		OrgID: 1, Name: "ci-app", Description: "d",
	}, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if plainSecret == "" || app.AppKey == "" {
		t.Fatal("expected key and one-time secret")
	}
	if app.AppSecret == plainSecret {
		t.Fatal("stored secret must be hashed")
	}
	if !verifyAppSecret(app.AppSecret, plainSecret) {
		t.Fatal("hash should verify")
	}

	// validate ok
	got, err := svc.ValidateApplication(context.Background(), app.AppKey, plainSecret)
	if err != nil || got.ID != app.ID {
		t.Fatalf("validate: %v %+v", err, got)
	}
	if len(repo.lastUsed) != 1 {
		t.Fatal("expected last_used update")
	}

	// wrong secret
	if _, err := svc.ValidateApplication(context.Background(), app.AppKey, "wrong"); err == nil {
		t.Fatal("wrong secret must fail")
	}

	// inactive
	inactive := false
	if err := svc.UpdateApplicationInOrg(context.Background(), app.ID, 1, &UpdateApplicationRequest{IsActive: &inactive}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateApplication(context.Background(), app.AppKey, plainSecret); err == nil {
		t.Fatal("inactive must fail")
	}
	// no-org path disabled
	if err := svc.UpdateApplication(context.Background(), app.ID, &UpdateApplicationRequest{Name: "x"}); err == nil {
		t.Fatal("UpdateApplication without org must fail")
	}
}

func TestApplicationService_ValidateExpired(t *testing.T) {
	repo := newMockAppRepo()
	svc := NewApplicationService(repo)
	past := time.Now().Add(-time.Hour)
	app, secret, err := svc.CreateApplication(context.Background(), &CreateApplicationRequest{
		OrgID: 1, Name: "exp", ExpiresAt: &past,
	}, "u")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateApplication(context.Background(), app.AppKey, secret); err == nil {
		t.Fatal("expired must fail")
	}
}

func TestApplicationService_RegenerateSecret(t *testing.T) {
	repo := newMockAppRepo()
	svc := NewApplicationService(repo)
	app, oldSecret, err := svc.CreateApplication(context.Background(), &CreateApplicationRequest{
		OrgID: 1, Name: "r",
	}, "u")
	if err != nil {
		t.Fatal(err)
	}
	newSecret, err := svc.RegenerateSecretInOrg(context.Background(), app.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if newSecret == "" || newSecret == oldSecret {
		t.Fatal("expected new secret")
	}
	if len(repo.regenCalls) != 1 {
		t.Fatal("regen not stored")
	}
	if !verifyAppSecret(repo.regenCalls[0].secret, newSecret) {
		t.Fatal("stored regen hash mismatch")
	}
	// old secret fails
	if _, err := svc.ValidateApplication(context.Background(), app.AppKey, oldSecret); err == nil {
		t.Fatal("old secret must fail after regen")
	}

	// inactive cannot regen
	off := false
	_ = svc.UpdateApplicationInOrg(context.Background(), app.ID, 1, &UpdateApplicationRequest{IsActive: &off})
	if _, err := svc.RegenerateSecretInOrg(context.Background(), app.ID, 1); err == nil {
		t.Fatal("inactive regen must fail")
	}
	if _, err := svc.RegenerateSecret(context.Background(), app.ID); err == nil {
		t.Fatal("RegenerateSecret without org must fail")
	}
}

func TestApplicationService_ListUpdateDelete(t *testing.T) {
	repo := newMockAppRepo()
	svc := NewApplicationService(repo)
	_, _, _ = svc.CreateApplication(context.Background(), &CreateApplicationRequest{OrgID: 1, Name: "a"}, "u")
	_, _, _ = svc.CreateApplication(context.Background(), &CreateApplicationRequest{OrgID: 2, Name: "b"}, "u")

	list, err := svc.ListApplications(context.Background(), 1, nil)
	if err != nil || len(list) != 1 {
		t.Fatalf("list org1: %v %d", err, len(list))
	}
	if err := svc.UpdateApplicationInOrg(context.Background(), list[0].ID, 1, &UpdateApplicationRequest{Name: "a2", Description: "d2"}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetApplicationInOrg(context.Background(), list[0].ID, 1)
	if err != nil || got.Name != "a2" {
		t.Fatalf("%+v %v", got, err)
	}
	if err := svc.DeleteApplicationInOrg(context.Background(), list[0].ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetApplicationInOrg(context.Background(), list[0].ID, 1); err == nil {
		t.Fatal("deleted")
	}
	if err := svc.DeleteApplication(context.Background(), list[0].ID); err == nil {
		t.Fatal("DeleteApplication without org must fail")
	}
}

func TestApplicationService_ValidateUnknownKey(t *testing.T) {
	svc := NewApplicationService(newMockAppRepo())
	if _, err := svc.ValidateApplication(context.Background(), "nope", "x"); err == nil {
		t.Fatal("unknown key")
	}
}

func TestVerifyAppSecret_Empty(t *testing.T) {
	if verifyAppSecret("", "x") || verifyAppSecret("x", "") {
		t.Fatal("empty must fail")
	}
}
