package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestWorkspaceController_GetWorkspaces(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	db := &gorm.DB{}
	workspaceController := NewWorkspaceController(
		services.NewWorkspaceService(db),
		services.NewWorkspaceOverviewService(db),
		nil,
	)
	router.GET("/workspaces", workspaceController.GetWorkspaces)

	req, _ := http.NewRequest("GET", "/workspaces", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, float64(200), response["code"])
	assert.Contains(t, response, "data")
}

func TestWorkspaceController_GetWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	db := &gorm.DB{}
	workspaceController := NewWorkspaceController(
		services.NewWorkspaceService(db),
		services.NewWorkspaceOverviewService(db),
		nil,
	)
	router.GET("/workspaces/:id", workspaceController.GetWorkspace)

	req, _ := http.NewRequest("GET", "/workspaces/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, float64(200), response["code"])
}
