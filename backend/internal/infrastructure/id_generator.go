package infrastructure

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"time"

	"iac-platform/internal/domain/valueobject"
)

// PermissionIDGenerator 权限ID生成器
type PermissionIDGenerator struct {
	mu      sync.Mutex
	counter uint64
}

// NewPermissionIDGenerator 创建权限ID生成器
func NewPermissionIDGenerator() *PermissionIDGenerator {
	return &PermissionIDGenerator{
		counter: 0,
	}
}

// Generate 生成业务语义ID
// 格式: {scope_prefix}pm-{timestamp}{counter}
// 例如: orgpm-1729756800001, wspm-1729756800002
func (g *PermissionIDGenerator) Generate(scopeType valueobject.ScopeType) string {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 获取作用域前缀
	prefix := g.getScopePrefix(scopeType)

	// 生成唯一ID: 时间戳(10位) + 计数器(6位)
	timestamp := time.Now().Unix()
	g.counter++
	if g.counter > 999999 {
		g.counter = 1
	}

	return fmt.Sprintf("%s-%d%06d", prefix, timestamp, g.counter)
}

// GenerateForResourceType 根据资源类型生成ID
func (g *PermissionIDGenerator) GenerateForResourceType(resourceType valueobject.ResourceType) string {
	scopeType := resourceType.GetScopeLevel()
	return g.Generate(scopeType)
}

// getScopePrefix 获取作用域前缀
func (g *PermissionIDGenerator) getScopePrefix(scopeType valueobject.ScopeType) string {
	switch scopeType {
	case valueobject.ScopeTypeOrganization:
		return "orgpm"
	case valueobject.ScopeTypeProject:
		return "pjpm"
	case valueobject.ScopeTypeWorkspace:
		return "wspm"
	default:
		return "pm"
	}
}

// GeneratePermissionID 生成权限定义ID（全局函数）
func GeneratePermissionID(resourceType valueobject.ResourceType) string {
	scopeType := resourceType.GetScopeLevel()
	prefix := ""

	switch scopeType {
	case valueobject.ScopeTypeOrganization:
		prefix = "orgpm"
	case valueobject.ScopeTypeProject:
		prefix = "pjpm"
	case valueobject.ScopeTypeWorkspace:
		prefix = "wspm"
	default:
		prefix = "pm"
	}

	// 使用时间戳 + 随机数生成唯一ID
	timestamp := time.Now().UnixNano() / 1000000 // 毫秒时间戳
	return fmt.Sprintf("%s-%d", prefix, timestamp)
}

// GenerateUserID 生成用户ID
// 格式: user-{10位随机小写字母+数字}
func GenerateUserID() (string, error) {
	return generateRandomID("user", 10)
}

// GenerateTeamID 生成团队ID
// 格式: team-{10位随机小写字母+数字}
func GenerateTeamID() (string, error) {
	return generateRandomID("team", 10)
}

// GenerateWorkspaceID 生成工作空间ID
// 格式: ws-{10位随机小写字母+数字}
func GenerateWorkspaceID() (string, error) {
	return generateRandomID("ws", 10)
}

// GenerateTokenID 生成Token ID
// 格式: token-{8-15位随机小写字母+数字}
func GenerateTokenID() (string, error) {
	// 生成8-15位之间的随机长度
	lengthRange := 8 // 15 - 8 + 1 = 8种可能
	lengthNum, err := rand.Int(rand.Reader, big.NewInt(int64(lengthRange)))
	if err != nil {
		return "", fmt.Errorf("failed to generate random length: %w", err)
	}
	length := 8 + int(lengthNum.Int64())

	return generateRandomID("token", length)
}

// GenerateSecretID 生成Secret ID
// 格式: secret-{16位随机小写字母+数字}
func GenerateSecretID() (string, error) {
	return generateRandomID("secret", 16)
}

// GenerateVariableID 生成Variable ID
// 格式: var-{16位随机小写字母+数字}
func GenerateVariableID() (string, error) {
	return generateRandomID("var", 16)
}

// generateRandomID 生成指定前缀和长度的随机ID
func generateRandomID(prefix string, length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))

	for i := range b {
		num, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}
		b[i] = charset[num.Int64()]
	}

	return fmt.Sprintf("%s-%s", prefix, string(b)), nil
}

// ValidateUserID 验证用户ID格式
func ValidateUserID(id string) bool {
	if len(id) != 15 { // "user-" (5) + 10位随机字符
		return false
	}
	if id[:5] != "user-" {
		return false
	}
	// 验证后10位是否都是小写字母或数字
	for _, c := range id[5:] {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// ValidateTeamID 验证团队ID格式
func ValidateTeamID(id string) bool {
	if len(id) != 15 { // "team-" (5) + 10位随机字符
		return false
	}
	if id[:5] != "team-" {
		return false
	}
	// 验证后10位是否都是小写字母或数字
	for _, c := range id[5:] {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// ValidateTokenID 验证Token ID格式
func ValidateTokenID(id string) bool {
	if len(id) < 14 || len(id) > 21 { // "token-" (6) + 8-15位随机字符
		return false
	}
	if id[:6] != "token-" {
		return false
	}
	// 验证后面的字符是否都是小写字母或数字
	for _, c := range id[6:] {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// ValidateSecretID 验证Secret ID格式
func ValidateSecretID(id string) bool {
	if len(id) != 23 { // "secret-" (7) + 16位随机字符
		return false
	}
	if id[:7] != "secret-" {
		return false
	}
	// 验证后16位是否都是小写字母或数字
	for _, c := range id[7:] {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}
