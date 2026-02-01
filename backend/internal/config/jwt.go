package config

import "os"

// GetJWTSecret 获取JWT密钥
// 优先从环境变量JWT_SECRET读取，如果为空则使用默认值
func GetJWTSecret() string {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "your-jwt-secret-key" // 默认密钥，生产环境应该设置环境变量
	}
	return secret
}
