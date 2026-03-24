package database

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// includeTempKey 用于在 context 中标记是否包含 temp 记录
type includeTempKey struct{}

// IncludeTempState 返回一个带有 include_temp 标记的 DB session。
// 使用此方法的查询将绕过全局 is_temp 过滤，能看到临时记录。
//
// 用法：db := database.IncludeTempState(a.getDB())
func IncludeTempState(db *gorm.DB) *gorm.DB {
	ctx := context.WithValue(db.Statement.Context, includeTempKey{}, true)
	return db.WithContext(ctx)
}

// RegisterStateVersionTempFilter 注册全局 GORM 回调，自动排除 workspace_state_versions 表中的临时记录。
//
// 效果：所有 SELECT 查询在涉及 workspace_state_versions 表时，自动附加 WHERE is_temp = false 条件。
// 绕过：使用 database.IncludeTempState(db) 获取不过滤 temp 的 session。
//
// 这确保 temp 记录对所有业务逻辑默认不可见，无需在每个查询点手动添加过滤条件。
func RegisterStateVersionTempFilter(db *gorm.DB) {
	db.Callback().Query().Before("gorm:query").Register("state_version:exclude_temp", func(db *gorm.DB) {
		if db.Statement.Schema == nil {
			return
		}
		if db.Statement.Schema.Table != "workspace_state_versions" {
			return
		}
		// 允许通过 IncludeTempState 绕过
		if v, ok := db.Statement.Context.Value(includeTempKey{}).(bool); ok && v {
			return
		}
		db.Statement.AddClause(clause.Where{
			Exprs: []clause.Expression{
				clause.Expr{SQL: "is_temp = ?", Vars: []interface{}{false}},
			},
		})
	})
}
