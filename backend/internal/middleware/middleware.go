package middleware

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"iac-platform/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
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
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")

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

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) > 0 {
			err := c.Errors.Last()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":      500,
				"message":   err.Error(),
				"timestamp": time.Now(),
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

			// 如果还没有token，尝试从query参数获取 (用于 WebSocket连接)
			if tokenString == "" {
				tokenString = c.Query("token")
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

		// 使用统一的JWT密钥解析token
		jwtSecret := config.GetJWTSecret()
		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			return []byte(jwtSecret), nil
		})

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

		// 兼容新旧格式的user_id
		userIDSet := false
		userIDValue := claims["user_id"]

		switch v := userIDValue.(type) {
		case string:
			// 新格式: string类型
			c.Set("user_id", v)
			userIDSet = true
		case float64:
			// 旧格式: 数字类型,转换为新格式
			c.Set("user_id", fmt.Sprintf("user-%d", uint(v)))
			userIDSet = true
		case int, int64, uint, uint64:
			// 其他数字类型
			c.Set("user_id", fmt.Sprintf("user-%v", v))
			userIDSet = true
		}

		if !userIDSet {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":      401,
				"message":   fmt.Sprintf("Invalid user_id in token, type: %T", userIDValue),
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		c.Set("username", claims["username"])

		// 检查token类型并验证
		tokenType, _ := claims["type"].(string)
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

			// 从数据库查询最新的role（确保管理员修改权限后立即生效，无需重新登录）
			var loginUser struct {
				Role     string
				IsActive bool
			}
			if err := globalDB.Table("users").Select("role, is_active").Where("user_id = ?", userID).First(&loginUser).Error; err != nil {
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
			c.Set("role", loginUser.Role)
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

			// 从数据库查询用户的role
			var user struct {
				Role     string
				IsActive bool
			}
			if err := globalDB.Table("users").Select("role, is_active").Where("user_id = ?", userID).First(&user).Error; err != nil {
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

			c.Set("role", user.Role)
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

			// 验证token在数据库中存在且有效（使用hash）
			var dbToken struct {
				TeamID   string
				IsActive bool
			}
			err := globalDB.Table("team_tokens").
				Select("team_id, is_active").
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

			// Team token不需要login session检查，可以长期使用
			// 设置team_id到context
			c.Set("team_id", dbToken.TeamID)

			// Team token使用团队的权限，不设置user role
			// 权限检查会基于team_id进行
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RequireRole 检查用户是否具有指定角色
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{
				"code":      403,
				"message":   "Access denied: role not found",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{
				"code":      403,
				"message":   "Access denied: invalid role format",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		for _, r := range roles {
			if roleStr == r {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{
			"code":      403,
			"message":   "Access denied: insufficient permissions",
			"timestamp": time.Now(),
		})
		c.Abort()
	}
}
