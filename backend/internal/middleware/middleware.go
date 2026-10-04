package middleware

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"iac-platform/internal/config"
	"iac-platform/internal/keys"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var globalDB *gorm.DB

// SetGlobalDB 设置全局数据库连接（用于JWT中间件查询用户信息）
func SetGlobalDB(db *gorm.DB) {
	globalDB = db
}

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, "+RequestIDHeader)
		// 跨域前端需能读到 ErrorHandler 回写的请求 ID
		c.Header("Access-Control-Expose-Headers", RequestIDHeader)

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func Logger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("%s - [%s] \"%s %s %s %d %s \"%s\" %s\"\n",
			param.ClientIP,
			param.TimeStamp.Format(time.RFC1123),
			param.Method,
			param.Path,
			param.Request.Proto,
			param.StatusCode,
			param.Latency,
			param.Request.UserAgent(),
			param.ErrorMessage,
		)
	})
}

// RequestIDHeader 请求 ID 头(请求可携带,响应总是回写)。
const RequestIDHeader = "X-Request-ID"

// RequestIDContextKey gin context 中的请求 ID。
const RequestIDContextKey = "request_id"

// validRequestID 只复用形如 [A-Za-z0-9-]{8,64} 的入站请求 ID(防日志/头注入)。
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

// ErrorHandler 统一 500 出口:handler 用 c.Error(err) 后 return,这里记录真实错误
// (日志带 request_id),响应只给通用信息,不泄露数据库 / SQL 文本。
// 每个请求都有 request_id:合法的入站 X-Request-ID 原样复用,否则生成 UUID;
// 写入响应头 X-Request-ID 与 500 响应体 request_id。
// InternalErrorResponse ErrorHandler 的通用 500 响应体(不含任何数据库 / SQL 文本)。
type InternalErrorResponse struct {
	Code      int       `json:"code" example:"500"`
	Error     string    `json:"error" example:"internal error"`
	Message   string    `json:"message" example:"Internal server error"`
	RequestID string    `json:"request_id" example:"3f2b8c1e-6a4d-4e2a-9c51-0d7e2f1a9b33"`
	Timestamp time.Time `json:"timestamp"`
}

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(RequestIDHeader)
		if !validRequestID.MatchString(requestID) {
			requestID = uuid.NewString()
		}
		c.Set(RequestIDContextKey, requestID)
		c.Header(RequestIDHeader, requestID)

		c.Next()

		if len(c.Errors) > 0 {
			err := c.Errors.Last()
			log.Printf("[Error] request_id=%s %s %s: %v", requestID, c.Request.Method, c.Request.URL.Path, err.Err)
			if c.Writer.Written() {
				return // handler 已自行响应
			}
			c.JSON(http.StatusInternalServerError, InternalErrorResponse{
				Code:      http.StatusInternalServerError,
				Error:     "internal error",
				Message:   "Internal server error",
				RequestID: requestID,
				Timestamp: time.Now(),
			})
		}
	}
}

func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")

		var tokenString string
		if authHeader != "" {
			tokenString = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			// 尝试从 Sec-WebSocket-Protocol 获取 token (用于 WebSocket)
			protocols := c.Request.Header.Get("Sec-WebSocket-Protocol")

			if protocols != "" {
				// 格式: "access_token, <token>"
				parts := strings.Split(protocols, ", ")
				if len(parts) >= 2 && parts[0] == "access_token" {
					tokenString = parts[1]
				}
			}

			if tokenString == "" {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":      401,
					"message":   "Authorization required",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}
		}

		// User-purpose signing key selected by kid (SIGNING_ROOT_KEY, current
		// or previous); tokens without kid only within the legacy window.
		token, err := jwt.Parse(tokenString,
			keys.Keyfunc(keys.PurposeUser, keys.LegacySecret(config.GetJWTSecret())),
			jwt.WithValidMethods([]string{"HS256"}))

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":      401,
				"message":   "Invalid token",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":      401,
				"message":   "Invalid token claims",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		// 先识别 token 类型：team_token 以 team 为主体，不要求 user_id
		tokenType, _ := claims["type"].(string)
		c.Set("token_type", tokenType)

		if tokenType != "team_token" {
			// 兼容新旧格式的 user_id（login / user token）
			userIDSet := false
			userIDValue := claims["user_id"]

			switch v := userIDValue.(type) {
			case string:
				c.Set("user_id", v)
				userIDSet = true
			case float64:
				c.Set("user_id", fmt.Sprintf("user-%d", uint(v)))
				userIDSet = true
			case int, int64, uint, uint64:
				c.Set("user_id", fmt.Sprintf("user-%v", v))
				userIDSet = true
			}

			if !userIDSet {
				log.Printf("[JWT] Invalid user_id type in token: %T", userIDValue)
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":      401,
					"message":   "Invalid token",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}

			c.Set("username", claims["username"])
			c.Set("principal_type", "USER")
			c.Set("principal_id", c.GetString("user_id"))
		}

		// 检查token类型并验证
		if tokenType == "login_token" {
			// Login token: 必须验证session_id在数据库中存在且有效
			sessionID, _ := claims["session_id"].(string)
			if sessionID == "" || globalDB == nil {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":      401,
					"message":   "Invalid login token: missing session_id",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}

			// 验证session在数据库中存在且有效
			var dbSession struct {
				UserID    string
				IsActive  bool
				ExpiresAt time.Time
			}
			userID := c.GetString("user_id")
			err := globalDB.Table("login_sessions").
				Select("user_id, is_active, expires_at").
				Where("session_id = ? AND user_id = ? AND is_active = ?", sessionID, userID, true).
				First(&dbSession).Error

			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":      401,
					"message":   "Invalid login token: session not found or revoked",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}

			// 检查session是否过期
			if dbSession.ExpiresAt.Before(time.Now()) {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":      401,
					"message":   "Login session has expired",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}

			// 更新最后使用时间
			now := time.Now()
			globalDB.Table("login_sessions").Where("session_id = ?", sessionID).Update("last_used_at", now)

			// 设置session_id到context（供logout使用）
			c.Set("session_id", sessionID)

			// 从数据库查询最新的用户状态（确保权限修改后立即生效，无需重新登录）
			var loginUser struct {
				IsActive      bool
				IsSystemAdmin bool
			}
			if err := globalDB.Table("users").Select("is_active, is_system_admin").Where("user_id = ?", userID).First(&loginUser).Error; err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":      401,
					"message":   "User not found",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}
			if !loginUser.IsActive {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":      401,
					"message":   "User is inactive",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}
			c.Set("is_system_admin", loginUser.IsSystemAdmin)
		} else if tokenType == "user_token" {
			// User token: 必须验证token_id在数据库中存在且有效
			tokenID, _ := claims["token_id"].(string)
			if tokenID == "" || globalDB == nil {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":      401,
					"message":   "Invalid user token: missing token_id",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}

			// 计算token_id的hash
			tokenIDHash := sha256.Sum256([]byte(tokenID))
			tokenIDHashStr := base64.StdEncoding.EncodeToString(tokenIDHash[:])

			// 验证token在数据库中存在且有效（使用hash）
			var dbToken struct {
				UserID   string
				IsActive bool
			}
			userID := c.GetString("user_id")
			err := globalDB.Table("user_tokens").
				Select("user_id, is_active").
				Where("token_id_hash = ? AND user_id = ? AND is_active = ?", tokenIDHashStr, userID, true).
				First(&dbToken).Error

			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":      401,
					"message":   "Invalid user token: token not found or revoked",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}

			// 检查用户是否有活跃的login session（增强安全：user token需要登录状态）
			var activeSessionCount int64
			globalDB.Table("login_sessions").
				Where("user_id = ? AND is_active = ? AND expires_at > ?", userID, true, time.Now()).
				Count(&activeSessionCount)

			if activeSessionCount == 0 {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":      401,
					"message":   "User token requires an active login session. Please login first.",
					"hint":      "User tokens can only be used when you have an active login session",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}

			// 从数据库查询用户状态
			var user struct {
				IsActive      bool
				IsSystemAdmin bool
			}
			if err := globalDB.Table("users").Select("is_active, is_system_admin").Where("user_id = ?", userID).First(&user).Error; err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":      401,
					"message":   "User not found",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}

			if !user.IsActive {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":      401,
					"message":   "User is inactive",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}

			c.Set("is_system_admin", user.IsSystemAdmin)
		} else if tokenType == "team_token" {
			// Team token: 必须验证token_id在数据库中存在且有效（不需要login session）
			tokenID, _ := claims["token_id"].(string)

			if tokenID == "" || globalDB == nil {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":      401,
					"message":   "Invalid team token: missing token_id",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}

			// 计算token_id的hash
			tokenIDHash := sha256.Sum256([]byte(tokenID))
			tokenIDHashStr := base64.StdEncoding.EncodeToString(tokenIDHash[:])

			// 验证 token 存在、活跃，并强制检查 DB expires_at（含旧无 JWT exp 的 token）
			var dbToken struct {
				TeamID    string
				IsActive  bool
				ExpiresAt *time.Time
			}
			err := globalDB.Table("team_tokens").
				Select("team_id, is_active, expires_at").
				Where("token_id_hash = ? AND is_active = ?", tokenIDHashStr, true).
				First(&dbToken).Error

			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":      401,
					"message":   "Invalid team token: token not found or revoked",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}
			// 禁止永不过期：expires_at 必须存在且未过期（B-2 / D4）
			if dbToken.ExpiresAt == nil {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":      401,
					"message":   "Team token has no expiry (forbidden); reissue required",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}
			if dbToken.ExpiresAt.Before(time.Now()) {
				// 惰性失效：标记 inactive 并拒绝
				globalDB.Table("team_tokens").
					Where("token_id_hash = ?", tokenIDHashStr).
					Updates(map[string]interface{}{
						"is_active":  false,
						"revoked_at": time.Now(),
						"revoked_by": "system:expired",
					})
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":      401,
					"message":   "Team token has expired",
					"timestamp": time.Now(),
				})
				c.Abort()
				return
			}

			// Team token：主体为 TEAM，不要求 login session
			c.Set("team_id", dbToken.TeamID)
			c.Set("principal_type", "TEAM")
			c.Set("principal_id", dbToken.TeamID)
			// IAM 中间件当前仍读 user_id；设置稳定合成主体，后续 checker 按 TEAM 求值
			c.Set("user_id", "team:"+dbToken.TeamID)
			c.Set("username", "team:"+dbToken.TeamID)
		} else {
			// 没有type字段的token - 拒绝访问（不再兼容旧格式）
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":      401,
				"message":   "Invalid token format: missing type field. Please login again to get a new token.",
				"hint":      "Old format tokens are no longer supported for security reasons",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireSystemAdmin 要求当前用户是系统管理员（is_system_admin=true）
// 用于系统级管理操作（如SSO配置），不走IAM权限体系
func RequireSystemAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isSystemAdmin, _ := c.Get("is_system_admin"); isSystemAdmin == true {
			c.Next()
			return
		}
		c.JSON(http.StatusForbidden, gin.H{
			"code":      403,
			"message":   "System admin access required",
			"timestamp": time.Now(),
		})
		c.Abort()
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

