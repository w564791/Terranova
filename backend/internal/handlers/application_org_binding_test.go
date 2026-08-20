package handlers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/entity"

	"github.com/gin-gonic/gin"
)

type memAppRepo struct {
	apps  map[uint]*entity.Application
	next  uint
	byKey map[string]*entity.Application
}

func newMemAppRepo() *memAppRepo {
	return &memAppRepo{apps: map[uint]*entity.Application{}, byKey: map[string]*entity.Application{}}
}
func (m *memAppRepo) Create(ctx context.Context, app *entity.Application) error {
	m.next++
	app.ID = m.next
	cp := *app
	m.apps[app.ID] = &cp
	m.byKey[app.AppKey] = &cp
	return nil
}
func (m *memAppRepo) GetByID(ctx context.Context, id uint) (*entity.Application, error) {
	a, ok := m.apps[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *a
	return &cp, nil
}
func (m *memAppRepo) GetByAppKey(ctx context.Context, k string) (*entity.Application, error) {
	a, ok := m.byKey[k]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *a
	return &cp, nil
}
func (m *memAppRepo) ListByOrg(ctx context.Context, orgID uint, isActive *bool) ([]*entity.Application, error) {
	var out []*entity.Application
	for _, a := range m.apps {
		if a.OrgID == orgID {
			cp := *a
			out = append(out, &cp)
		}
	}
	return out, nil
}
func (m *memAppRepo) Update(ctx context.Context, app *entity.Application) error {
	m.apps[app.ID] = app
	return nil
}
func (m *memAppRepo) Delete(ctx context.Context, id uint) error {
	delete(m.apps, id)
	return nil
}
func (m *memAppRepo) UpdateLastUsed(ctx context.Context, id uint) error { return nil }
func (m *memAppRepo) RegenerateSecret(ctx context.Context, id uint, secret string) error {
	if a, ok := m.apps[id]; ok {
		a.AppSecret = secret
	}
	return nil
}

func appRouter(h *ApplicationHandler, authOrg uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Set("auth_org_id", authOrg)
		c.Next()
	})
	r.POST("/applications", h.CreateApplication)
	r.GET("/applications", h.ListApplications)
	r.GET("/applications/:id", h.GetApplication)
	r.DELETE("/applications/:id", h.DeleteApplication)
	r.POST("/applications/:id/regenerate-secret", h.RegenerateSecret)
	return r
}

func TestApplicationHandler_CreateRejectsCrossOrg(t *testing.T) {
	svc := service.NewApplicationService(newMemAppRepo())
	h := NewApplicationHandler(svc)
	r := appRouter(h, 1)
	body := `{"org_id":2,"name":"evil"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/applications", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d %s", w.Code, w.Body.String())
	}
}

func TestApplicationHandler_GetCrossOrg404(t *testing.T) {
	repo := newMemAppRepo()
	svc := service.NewApplicationService(repo)
	// seed app in org 2
	_, _, err := svc.CreateApplication(context.Background(), &service.CreateApplicationRequest{OrgID: 2, Name: "a"}, "u")
	if err != nil {
		t.Fatal(err)
	}
	h := NewApplicationHandler(svc)
	r := appRouter(h, 1)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/applications/1", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d %s", w.Code, w.Body.String())
	}
}

func TestApplicationHandler_ListCrossOrgQueryForbidden(t *testing.T) {
	svc := service.NewApplicationService(newMemAppRepo())
	h := NewApplicationHandler(svc)
	r := appRouter(h, 1)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/applications?org_id=2", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d %s", w.Code, w.Body.String())
	}
}

func TestApplicationHandler_UpdateDeleteRegenerate_CrossOrg(t *testing.T) {
	repo := newMemAppRepo()
	svc := service.NewApplicationService(repo)
	_, _, err := svc.CreateApplication(context.Background(), &service.CreateApplicationRequest{OrgID: 2, Name: "other"}, "u")
	if err != nil {
		t.Fatal(err)
	}
	h := NewApplicationHandler(svc)
	r := appRouter(h, 1)
	r.PUT("/applications/:id", h.UpdateApplication)

	// update cross org
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/applications/1", bytes.NewBufferString(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("update cross org want 404 got %d %s", w.Code, w.Body.String())
	}

	// regenerate cross org
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest("POST", "/applications/1/regenerate-secret", nil))
	if w2.Code != http.StatusNotFound {
		t.Fatalf("regenerate cross org want 404 got %d %s", w2.Code, w2.Body.String())
	}

	// delete cross org
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, httptest.NewRequest("DELETE", "/applications/1", nil))
	if w3.Code != http.StatusNotFound {
		t.Fatalf("delete cross org want 404 got %d %s", w3.Code, w3.Body.String())
	}
}

func TestApplicationService_NoOrgAPIsDisabled(t *testing.T) {
	svc := service.NewApplicationService(newMemAppRepo())
	if _, err := svc.GetApplication(context.Background(), 1); !errors.Is(err, service.ErrApplicationOrgForbidden) {
		t.Fatalf("GetApplication: %v", err)
	}
	if err := svc.UpdateApplication(context.Background(), 1, &service.UpdateApplicationRequest{Name: "x"}); !errors.Is(err, service.ErrApplicationOrgForbidden) {
		t.Fatalf("UpdateApplication: %v", err)
	}
	if err := svc.DeleteApplication(context.Background(), 1); !errors.Is(err, service.ErrApplicationOrgForbidden) {
		t.Fatalf("DeleteApplication: %v", err)
	}
	if _, err := svc.RegenerateSecret(context.Background(), 1); !errors.Is(err, service.ErrApplicationOrgForbidden) {
		t.Fatalf("RegenerateSecret: %v", err)
	}
}

func TestApplicationHandler_CreateAndGetSameOrg(t *testing.T) {
	svc := service.NewApplicationService(newMemAppRepo())
	h := NewApplicationHandler(svc)
	r := appRouter(h, 1)
	body := `{"org_id":1,"name":"ok-app"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/applications", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest("GET", "/applications/1", nil))
	if w2.Code != 200 {
		t.Fatalf("get: %d %s", w2.Code, w2.Body.String())
	}
}
