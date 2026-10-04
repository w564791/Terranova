package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// OverrideView 对外输出的单个覆盖(deployment 与任务快照共用)。
// Value 仅在 key 非敏感且调用者持有该 workspace WORKSPACE_VARIABLES READ 时出现。
type OverrideView struct {
	Key       string  `json:"key"`
	Sensitive bool    `json:"sensitive"`
	HasValue  bool    `json:"has_value"`
	Value     *string `json:"value,omitempty"`
}

// ParseOverrides 把 jsonb 覆盖解成扁平 key -> string(非字符串值按 %v 格式化,
// 与 GetActiveDeploymentExtras 同一规则)。
func ParseOverrides(raw []byte) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return out
	}
	return FlattenOverrides(m)
}

// FlattenOverrides 把 map 形式的覆盖(任务行 JSONB)解成扁平 key -> string。
func FlattenOverrides(m map[string]interface{}) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if sv, ok := v.(string); ok {
			out[k] = sv
		} else if v != nil {
			out[k] = fmt.Sprintf("%v", v)
		} else {
			out[k] = ""
		}
	}
	return out
}

// ParseSensitiveKeys 解析 sensitive_keys 列。known=false 表示 NULL / 无法解析:
// 尚未计算,调用方必须按"全部敏感"处理。
func ParseSensitiveKeys(raw json.RawMessage) (keys map[string]bool, known bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, false
	}
	keys = make(map[string]bool, len(list))
	for _, k := range list {
		keys[k] = true
	}
	return keys, true
}

// EncodeSensitiveKeys 编码为排序后的 jsonb 字符串数组(空集合为 [] 而不是 NULL)。
func EncodeSensitiveKeys(keys map[string]bool) json.RawMessage {
	list := make([]string, 0, len(keys))
	for k, v := range keys {
		if v {
			list = append(list, k)
		}
	}
	sort.Strings(list)
	b, _ := json.Marshal(list)
	return b
}

// RedactOverrides 是所有返回部署覆盖 / 任务覆盖快照的接口唯一的输出口:
//   - sensitive_keys 为 NULL(未计算)=> 所有 key 视为敏感,不返回任何值;
//   - 敏感 key 永不带 value;
//   - canReadValues=false(调用者无该 workspace 的 WORKSPACE_VARIABLES READ)=> 不带任何 value。
func RedactOverrides(overrides map[string]string, sensitiveKeys json.RawMessage, canReadValues bool) []OverrideView {
	sens, known := ParseSensitiveKeys(sensitiveKeys)
	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]OverrideView, 0, len(keys))
	for _, k := range keys {
		v := overrides[k]
		item := OverrideView{Key: k, Sensitive: !known || sens[k], HasValue: v != ""}
		if !item.Sensitive && canReadValues {
			val := v
			item.Value = &val
		}
		out = append(out, item)
	}
	return out
}

// ComputeDeploymentSensitiveKeys 计算一次部署的敏感 key(install / upgrade / 预览 /
// 启动回填共用),任一来源即敏感:
//   - versionIDs 中各版本 manifest_files 顶层 variable 块的 sensitive = true
//     (ParseManifestVariables,与发布时提取变量同一解析);
//   - 目标 workspace 解析链 + varsetIDs 中同名的敏感变量
//     (ResolveDisplayWithExtra 的 sensitive 标记,与变量预览同一判定)。
//
// 请求里的 sensitive 标记与已存 sensitive_keys 的粘滞合并由调用方决定。
func ComputeDeploymentSensitiveKeys(db *gorm.DB, versionIDs []string, workspaceID string, varsetIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	seen := map[string]bool{}
	for _, vid := range versionIDs {
		if vid == "" || seen[vid] {
			continue
		}
		seen[vid] = true
		var rows []models.ManifestFile
		if err := db.Select("path, content").Where("version_id = ?", vid).Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("load files of version %s: %w", vid, err)
		}
		scope := make(map[string][]byte, len(rows))
		for _, r := range rows {
			scope[r.Path] = r.Content
		}
		for _, m := range ParseManifestVariables(scope) {
			if m.Sensitive {
				out[m.Name] = true
			}
		}
	}
	resolved, err := NewVariableResolutionService(db).ResolveDisplayWithExtra(workspaceID, varsetIDs, nil)
	if err != nil {
		return nil, fmt.Errorf("resolve variable sensitivity: %w", err)
	}
	for _, v := range resolved {
		if v.Sensitive {
			out[v.Key] = true
		}
	}
	return out, nil
}

// BackfillDeploymentSensitiveKeys 为 sensitive_keys 仍为 NULL 的部署写入
// ComputeDeploymentSensitiveKeys 的结果(部署版本 + 部署 varsets)。幂等:只写
// 仍为 NULL 的行(WHERE sensitive_keys IS NULL,不覆盖并发 install/upgrade 写入的值),
// 不做批量全标记。单行失败记日志并保持 NULL(API 仍按全部敏感处理),下次启动重试。
func BackfillDeploymentSensitiveKeys(ctx context.Context, db *gorm.DB) (updated int, err error) {
	var deps []models.ManifestDeployment
	if err := db.WithContext(ctx).Select("id, version_id, workspace_id").
		Where("sensitive_keys IS NULL").Order("id").Find(&deps).Error; err != nil {
		return 0, fmt.Errorf("list deployments without sensitive_keys: %w", err)
	}
	failed := 0
	for _, d := range deps {
		if ctx.Err() != nil {
			return updated, ctx.Err()
		}
		var varsetIDs []string
		if err := db.WithContext(ctx).Model(&models.ManifestDeploymentVarset{}).
			Where("deployment_id = ?", d.ID).Order("priority ASC").
			Pluck("varset_id", &varsetIDs).Error; err != nil {
			log.Printf("[sensitive_keys backfill] deployment %s: load varsets: %v", d.ID, err)
			failed++
			continue
		}
		keys, err := ComputeDeploymentSensitiveKeys(db.WithContext(ctx), []string{d.VersionID}, d.WorkspaceID, varsetIDs)
		if err != nil {
			log.Printf("[sensitive_keys backfill] deployment %s: %v", d.ID, err)
			failed++
			continue
		}
		res := db.WithContext(ctx).Model(&models.ManifestDeployment{}).
			Where("id = ? AND sensitive_keys IS NULL", d.ID).
			UpdateColumn("sensitive_keys", EncodeSensitiveKeys(keys))
		if res.Error != nil {
			log.Printf("[sensitive_keys backfill] deployment %s: write: %v", d.ID, res.Error)
			failed++
			continue
		}
		updated += int(res.RowsAffected)
	}
	if failed > 0 {
		return updated, fmt.Errorf("%d deployment(s) left without sensitive_keys", failed)
	}
	return updated, nil
}
