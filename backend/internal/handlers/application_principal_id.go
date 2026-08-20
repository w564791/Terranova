package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// resolveApplicationPrincipalID 将管理台传入的 APPLICATION principal_id 规范为 app_key。
// 支持：
//   - 已是 app_key（非纯数字）→ 原样返回（并校验存在）
//   - 数字主键 / 数字字符串 → 查 applications.id 取 app_key
//
// 运行时 AgentAuth 写 principal_id=app_key；授权侧必须一致（选项 A）。
func resolveApplicationPrincipalID(ctx context.Context, db *gorm.DB, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("application principal_id is required")
	}
	if db == nil {
		return "", fmt.Errorf("db not configured for application principal resolve")
	}

	// 纯数字 → 按主键
	if id, err := strconv.ParseUint(raw, 10, 64); err == nil && id > 0 {
		var appKey string
		if err := db.WithContext(ctx).Table("applications").
			Select("app_key").Where("id = ?", id).Scan(&appKey).Error; err != nil || appKey == "" {
			return "", fmt.Errorf("application id %d not found", id)
		}
		return appKey, nil
	}

	// 已是 app_key
	var n int64
	if err := db.WithContext(ctx).Table("applications").
		Where("app_key = ?", raw).Count(&n).Error; err != nil {
		return "", err
	}
	if n == 0 {
		return "", fmt.Errorf("application app_key %q not found", raw)
	}
	return raw, nil
}
