package valueobject

import "fmt"

// ScopeType 作用域类型
type ScopeType string

const (
	// ScopeTypeOrganization 组织级作用域
	ScopeTypeOrganization ScopeType = "ORGANIZATION"
	// ScopeTypeProject 项目级作用域
	ScopeTypeProject ScopeType = "PROJECT"
	// ScopeTypeWorkspace 工作空间级作用域
	ScopeTypeWorkspace ScopeType = "WORKSPACE"
)

// String 返回作用域类型的字符串表示
func (s ScopeType) String() string {
	return string(s)
}

// IsValid 验证作用域类型是否有效
func (s ScopeType) IsValid() bool {
	switch s {
	case ScopeTypeOrganization, ScopeTypeProject, ScopeTypeWorkspace:
		return true
	default:
		return false
	}
}

// ParseScopeType 从字符串解析作用域类型
func ParseScopeType(s string) (ScopeType, error) {
	scopeType := ScopeType(s)
	if !scopeType.IsValid() {
		return "", fmt.Errorf("invalid scope type: %s", s)
	}
	return scopeType, nil
}

// GetPriority 获取作用域优先级（数字越大优先级越高）
// Workspace(3) > Project(2) > Organization(1)
func (s ScopeType) GetPriority() int {
	switch s {
	case ScopeTypeWorkspace:
		return 3
	case ScopeTypeProject:
		return 2
	case ScopeTypeOrganization:
		return 1
	default:
		return 0
	}
}

// IsMoreSpecificThan 判断当前作用域是否比目标作用域更精确
func (s ScopeType) IsMoreSpecificThan(target ScopeType) bool {
	return s.GetPriority() > target.GetPriority()
}

// CanHostPolicyFor reports whether a Role policy may be assigned at s for a
// resource whose native scope is resourceScope. A policy scope describes the
// scope of the Role assignment, not the physical scope of the resource. For
// example, a WORKSPACE_EXECUTION policy may be granted to a Role at
// ORGANIZATION, PROJECT, or WORKSPACE scope; an ORGANIZATION resource may be
// granted only at ORGANIZATION scope. Allowing the inverse direction would
// make a narrow object scope govern a broader parent resource.
func (s ScopeType) CanHostPolicyFor(resourceScope ScopeType) bool {
	return s.IsValid() && resourceScope.IsValid() && s.GetPriority() <= resourceScope.GetPriority()
}
