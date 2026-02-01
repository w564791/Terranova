package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"sync"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/infrastructure/persistence"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AuditConfig 审计配置缓存
type AuditConfig struct {
	Enabled        bool
	IncludeBody    bool
	IncludeHeaders bool
	mu             sync.RWMutex
	lastUpdate     time.Time
}

var (
	auditConfigCache = &AuditConfig{
		Enabled:        true, // 默认启用
		IncludeBody:    false,
		IncludeHeaders: false,
	}
	configRefreshInterval = 1 * time.Minute
)

// AuditLogger 审计日志中间件
func AuditLogger(db *gorm.DB) gin.HandlerFunc {
	auditRepo := persistence.NewAuditRepository(db)

	// 启动配置刷新goroutine
	go startConfigRefresher(db)

	return func(c *gin.Context) {
		// 从缓存检查审计日志是否启用
		if !auditConfigCache.IsEnabled() {
			c.Next()
			return
		}

		// 记录开始时间（使用本地时间）
		startTime := time.Now()

		// 获取用户ID
		userID, exists := c.Get("user_id")
		var uid string
		if exists {
			uid = userID.(string)
		}

		// 获取请求信息
		method := c.Request.Method
		path := c.Request.URL.Path
		ip := c.ClientIP()
		userAgent := c.Request.UserAgent()

		// 从缓存检查是否需要记录请求体和请求头
		includeBody := auditConfigCache.ShouldIncludeBody()
		includeHeaders := auditConfigCache.ShouldIncludeHeaders()

		// 读取请求体（可选）
		var requestBody string
		if includeBody && c.Request.Body != nil && method != "GET" {
			bodyBytes, _ := c.GetRawData()
			requestBody = string(bodyBytes)
			// 重新设置body供后续handler使用
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// 获取请求头（可选）并转换为JSON字符串，移除敏感信息
		var requestHeaders string
		if includeHeaders {
			requestHeadersMap := make(map[string]interface{})
			for key, values := range c.Request.Header {
				// 跳过敏感的header
				if key == "Authorization" || key == "Cookie" {
					continue
				}
				if len(values) > 0 {
					requestHeadersMap[key] = values[0]
				}
			}
			requestHeadersJSON, _ := json.Marshal(requestHeadersMap)
			requestHeaders = string(requestHeadersJSON)
		} else {
			// 如果不记录请求头，设置为null以避免jsonb类型错误
			requestHeaders = "null"
		}

		// 处理请求
		c.Next()

		// 计算耗时
		duration := time.Since(startTime)
		durationMs := int(duration.Milliseconds())

		// 获取响应状态
		statusCode := c.Writer.Status()
		isAllowed := statusCode >= 200 && statusCode < 400

		// 确定资源类型和操作
		resourceType := determineResourceType(path)
		action := method

		// 记录访问日志（仅记录IAM相关的API）
		shouldLog := shouldLogRequest(path)

		if shouldLog && uid != "" {
			accessLog := &entity.AccessLog{
				UserID:         uid,
				ResourceType:   resourceType,
				ResourceID:     0, // 可以从路径中提取
				Action:         action,
				IsAllowed:      isAllowed,
				DenyReason:     getDenyReason(c, statusCode),
				AccessedAt:     startTime,
				IPAddress:      ip,
				UserAgent:      userAgent,
				RequestPath:    path,
				HttpCode:       statusCode,
				RequestBody:    requestBody,
				RequestHeaders: requestHeaders,
				DurationMs:     durationMs,
			}

			// 同步记录日志
			if err := auditRepo.LogResourceAccess(c.Request.Context(), accessLog); err != nil {
				// 记录错误但不影响请求
				log.Printf("[AuditLogger] ERROR: Failed to record audit log: %v", err)
				c.Set("audit_log_error", err.Error())
			}
		}
	}
}

// IsEnabled 检查审计日志是否启用（从缓存读取）
func (ac *AuditConfig) IsEnabled() bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.Enabled
}

// ShouldIncludeBody 检查是否应该记录请求体（从缓存读取）
func (ac *AuditConfig) ShouldIncludeBody() bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.IncludeBody
}

// ShouldIncludeHeaders 检查是否应该记录请求头（从缓存读取）
func (ac *AuditConfig) ShouldIncludeHeaders() bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.IncludeHeaders
}

// Update 更新配置（写锁）
func (ac *AuditConfig) Update(enabled, includeBody, includeHeaders bool) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.Enabled = enabled
	ac.IncludeBody = includeBody
	ac.IncludeHeaders = includeHeaders
	ac.lastUpdate = time.Now()
}

// startConfigRefresher 启动配置刷新器
func startConfigRefresher(db *gorm.DB) {
	// 立即加载一次配置
	refreshConfig(db)

	// 定期刷新配置
	ticker := time.NewTicker(configRefreshInterval)
	defer ticker.Stop()

	for range ticker.C {
		refreshConfig(db)
	}
}

// refreshConfig 从数据库刷新配置到缓存
func refreshConfig(db *gorm.DB) {
	// 一次性查询所有需要的配置，减少数据库往返
	type ConfigRow struct {
		Key   string `gorm:"column:key"`
		Value string `gorm:"column:value"`
	}
	
	var configs []ConfigRow
	err := db.Table("system_configs").
		Select("key, value::text as value").
		Where("key IN ?", []string{
			"audit_log_enabled",
			"audit_log_include_body",
			"audit_log_include_headers",
		}).
		Scan(&configs).Error

	if err != nil {
		log.Printf("[AuditConfig] Failed to load configs: %v, using defaults", err)
		auditConfigCache.Update(true, false, false)
		return
	}

	// 解析配置值
	configMap := make(map[string]bool)
	for _, cfg := range configs {
		// 去掉JSON字符串的引号
		value := cfg.Value
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}
		configMap[cfg.Key] = value == "true"
	}

	// 使用默认值填充缺失的配置
	enabled := getConfigValue(configMap, "audit_log_enabled", true)
	includeBody := getConfigValue(configMap, "audit_log_include_body", false)
	includeHeaders := getConfigValue(configMap, "audit_log_include_headers", false)

	auditConfigCache.Update(enabled, includeBody, includeHeaders)
	// log.Printf("[AuditConfig] Refreshed: enabled=%v, includeBody=%v, includeHeaders=%v", enabled, includeBody, includeHeaders)
}

// getConfigValue 从配置映射中获取值，如果不存在则返回默认值
func getConfigValue(configMap map[string]bool, key string, defaultValue bool) bool {
	if value, exists := configMap[key]; exists {
		return value
	}
	return defaultValue
}

// shouldLogRequest 判断是否应该记录该请求
func shouldLogRequest(path string) bool {
	// 只记录IAM相关的API请求
	if len(path) >= 12 && path[:12] == "/api/v1/iam/" {
		return true
	}
	// 也可以记录其他重要的API
	if len(path) >= 19 && path[:19] == "/api/v1/workspaces/" {
		return true
	}
	return false
}

// determineResourceType 根据路径确定资源类型
func determineResourceType(path string) string {
	if len(path) < 12 {
		return "UNKNOWN"
	}

	// IAM相关资源
	if len(path) >= 12 && path[:12] == "/api/v1/iam/" {
		if contains(path, "/organizations") {
			return "ORGANIZATION"
		}
		if contains(path, "/projects") {
			return "PROJECT"
		}
		if contains(path, "/teams") {
			return "TEAM"
		}
		if contains(path, "/permissions") {
			return "PERMISSION"
		}
		if contains(path, "/applications") {
			return "APPLICATION"
		}
		if contains(path, "/users") {
			return "USER"
		}
		if contains(path, "/audit") {
			return "AUDIT_LOG"
		}
		// 默认IAM路径下的其他资源类型
		return "IAM"
	}

	// Workspace相关资源
	if contains(path, "/workspaces") {
		return "WORKSPACE"
	}

	return "UNKNOWN"
}

// getDenyReason 获取拒绝原因
func getDenyReason(c *gin.Context, statusCode int) string {
	if statusCode >= 200 && statusCode < 400 {
		return ""
	}

	// 从响应中获取错误信息
	if err, exists := c.Get("error"); exists {
		return err.(string)
	}

	// 根据状态码返回默认原因
	switch statusCode {
	case 401:
		return "未授权"
	case 403:
		return "权限不足"
	case 404:
		return "资源不存在"
	case 500:
		return "服务器内部错误"
	default:
		return "请求失败"
	}
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
