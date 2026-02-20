package valueobject

import "fmt"

// PermissionLevel 权限等级
type PermissionLevel int

const (
	// PermissionLevelNone 显式拒绝（最高优先级）
	PermissionLevelNone PermissionLevel = 0
	// PermissionLevelRead 只读权限
	PermissionLevelRead PermissionLevel = 1
	// PermissionLevelWrite 读写权限
	PermissionLevelWrite PermissionLevel = 2
	// PermissionLevelAdmin 管理权限
	PermissionLevelAdmin PermissionLevel = 3
)

// String 返回权限等级的字符串表示
func (p PermissionLevel) String() string {
	switch p {
	case PermissionLevelNone:
		return "NONE"
	case PermissionLevelRead:
		return "READ"
	case PermissionLevelWrite:
		return "WRITE"
	case PermissionLevelAdmin:
		return "ADMIN"
	default:
		return "UNKNOWN"
	}
}

// IsValid 验证权限等级是否有效
func (p PermissionLevel) IsValid() bool {
	return p >= PermissionLevelNone && p <= PermissionLevelAdmin
}

// GreaterThanOrEqual 判断是否大于等于目标等级
func (p PermissionLevel) GreaterThanOrEqual(target PermissionLevel) bool {
	return p >= target
}

// ParsePermissionLevel 从字符串解析权限等级
func ParsePermissionLevel(s string) (PermissionLevel, error) {
	switch s {
	case "NONE":
		return PermissionLevelNone, nil
	case "READ":
		return PermissionLevelRead, nil
	case "WRITE":
		return PermissionLevelWrite, nil
	case "ADMIN":
		return PermissionLevelAdmin, nil
	default:
		return PermissionLevelNone, fmt.Errorf("invalid permission level: %s", s)
	}
}

// Max 返回两个权限等级中的最大值
func Max(a, b PermissionLevel) PermissionLevel {
	if a > b {
		return a
	}
	return b
}

// MaxLevel 返回权限等级列表中的最大值
func MaxLevel(levels []PermissionLevel) PermissionLevel {
	if len(levels) == 0 {
		return PermissionLevelNone
	}

	maxLevel := levels[0]
	for _, level := range levels[1:] {
		if level > maxLevel {
			maxLevel = level
		}
	}
	return maxLevel
}

// MarshalJSON 实现JSON序列化
func (p PermissionLevel) MarshalJSON() ([]byte, error) {
	return []byte(`"` + p.String() + `"`), nil
}

// UnmarshalJSON 实现JSON反序列化
func (p *PermissionLevel) UnmarshalJSON(data []byte) error {
	// 移除引号
	str := string(data)
	if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
		str = str[1 : len(str)-1]
	}

	level, err := ParsePermissionLevel(str)
	if err != nil {
		return err
	}

	*p = level
	return nil
}
