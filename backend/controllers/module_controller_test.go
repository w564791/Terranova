package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"iac-platform/services"
	"gorm.io/gorm"
)

func TestModuleController_GetModules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	// 创建测试路由
	router := gin.New()
	moduleController := NewModuleController(services.NewModuleService(&gorm.DB{}))
	router.GET("/modules", moduleController.GetModules)

	// 创建测试请求
	req, _ := http.NewRequest("GET", "/modules", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)
	
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, float64(200), response["code"])
	assert.Contains(t, response, "data")
}

func TestModuleController_GetModule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	router := gin.New()
	moduleController := NewModuleController(services.NewModuleService(&gorm.DB{}))
	router.GET("/modules/:id", moduleController.GetModule)

	req, _ := http.NewRequest("GET", "/modules/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, float64(200), response["code"])
}