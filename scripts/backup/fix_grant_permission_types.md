# GrantPermission.tsx 需要修复的地方

## 问题
1. 第45行: `interface Team { id: number; }` 应改为 `id: string | number;`
2. 第95行: `principal_id: urlPrincipalId ? Number(urlPrincipalId) : 0` 应改为直接使用 string
3. 第305行和318行: `Number(e.target.value)` 需要根据 principal_type 判断类型
4. principal_id 状态应该是 `string | number` 类型
5. 验证逻辑需要调整

## 建议
由于这个页面涉及多种主体类型(USER/TEAM/APPLICATION),建议:
1. principal_id 使用 `string | number` 类型
2. 从 URL 解析时不转换类型,保持原始字符串
3. 在提交时根据实际类型处理

这是一个较大的前端重构,建议作为独立任务处理。
