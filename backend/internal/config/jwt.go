package config

import "os"

// GetJWTSecret 获取JWT密钥
// 从环境变量JWT_SECRET读取，未设置则 panic
func GetJWTSecret() string {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		panic("JWT_SECRET environment variable is required but not set")
	}
	return secret
}
