package valueobject

import "fmt"

// PrincipalType 主体类型（权限被授予的对象）
type PrincipalType string

const (
	// PrincipalTypeUser 用户
	PrincipalTypeUser PrincipalType = "USER"
	// PrincipalTypeTeam 团队
	PrincipalTypeTeam PrincipalType = "TEAM"
	// PrincipalTypeApplication 应用（Agent/外部系统）
	PrincipalTypeApplication PrincipalType = "APPLICATION"
)

// String 返回主体类型的字符串表示
func (p PrincipalType) String() string {
	return string(p)
}

// IsValid 验证主体类型是否有效
func (p PrincipalType) IsValid() bool {
	switch p {
	case PrincipalTypeUser, PrincipalTypeTeam, PrincipalTypeApplication:
		return true
	default:
		return false
	}
}

// ParsePrincipalType 从字符串解析主体类型
func ParsePrincipalType(s string) (PrincipalType, error) {
	principalType := PrincipalType(s)
	if !principalType.IsValid() {
		return "", fmt.Errorf("invalid principal type: %s", s)
	}
	return principalType, nil
}

// IsUser 判断是否为用户类型
func (p PrincipalType) IsUser() bool {
	return p == PrincipalTypeUser
}

// IsTeam 判断是否为团队类型
func (p PrincipalType) IsTeam() bool {
	return p == PrincipalTypeTeam
}

// IsApplication 判断是否为应用类型
func (p PrincipalType) IsApplication() bool {
	return p == PrincipalTypeApplication
}

// CanBeGrantedAt 判断该主体类型是否可以在指定作用域被授予权限
// Application只能在Organization级别被授予权限
func (p PrincipalType) CanBeGrantedAt(scope ScopeType) bool {
	if p == PrincipalTypeApplication {
		return scope == ScopeTypeOrganization
	}
	// User和Team可以在任何级别被授予权限
	return true
}
