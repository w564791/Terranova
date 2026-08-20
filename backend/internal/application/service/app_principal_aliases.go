package service

import (
	"context"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// ApplicationPrincipalAliases 将 APPLICATION principal_id 展开为可匹配的 id 列表（app_key + 数字主键）
// 用于兼容历史 grant 存 app.id、运行时用 app_key 的情况（选项 A）。
type ApplicationPrincipalAliases interface {
	ExpandApplicationPrincipalIDs(ctx context.Context, principalID string) ([]string, error)
}

// DBApplicationPrincipalAliases 基于 applications 表展开
type DBApplicationPrincipalAliases struct {
	db *gorm.DB
}

// NewDBApplicationPrincipalAliases 创建
func NewDBApplicationPrincipalAliases(db *gorm.DB) *DBApplicationPrincipalAliases {
	return &DBApplicationPrincipalAliases{db: db}
}

// ExpandApplicationPrincipalIDs 返回至少包含输入本身的 id 列表
func (a *DBApplicationPrincipalAliases) ExpandApplicationPrincipalIDs(ctx context.Context, principalID string) ([]string, error) {
	principalID = strings.TrimSpace(principalID)
	if principalID == "" {
		return nil, nil
	}
	// 去掉 app: 前缀（user_id 合成形态）
	if strings.HasPrefix(principalID, "app:") {
		principalID = strings.TrimPrefix(principalID, "app:")
	}
	out := []string{principalID}
	if a == nil || a.db == nil {
		return out, nil
	}

	var row struct {
		ID     uint   `gorm:"column:id"`
		AppKey string `gorm:"column:app_key"`
	}

	if id, err := strconv.ParseUint(principalID, 10, 64); err == nil && id > 0 {
		if err := a.db.WithContext(ctx).Table("applications").
			Select("id, app_key").Where("id = ?", id).Take(&row).Error; err == nil {
			if row.AppKey != "" && row.AppKey != principalID {
				out = append(out, row.AppKey)
			}
		}
		return uniqueStrings(out), nil
	}

	if err := a.db.WithContext(ctx).Table("applications").
		Select("id, app_key").Where("app_key = ?", principalID).Take(&row).Error; err == nil {
		if row.ID > 0 {
			idStr := strconv.FormatUint(uint64(row.ID), 10)
			if idStr != principalID {
				out = append(out, idStr)
			}
		}
	}
	return uniqueStrings(out), nil
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
