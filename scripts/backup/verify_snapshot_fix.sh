#!/bin/bash

# 验证变量快照修复是否生效

echo "=== 验证变量快照修复 ==="
echo ""

# 1. 检查代码文件的修改时间
echo "1. 代码文件修改时间:"
ls -la backend/controllers/workspace_task_controller.go | awk '{print "   ", $6, $7, $8}'
ls -la backend/services/terraform_executor.go | awk '{print "   ", $6, $7, $8}'
echo ""

# 2. 创建新任务
echo "2. 请手动创建一个新任务..."
echo "   等待任务创建完成后按回车继续"
read

# 3. 查询最新任务的快照
echo "3. 查询最新任务的快照格式:"
docker exec iac-platform-postgres psql -U postgres -d iac_platform -c \
  "SELECT id, created_at, 
   length(snapshot_variables::text) as len,
   substring(snapshot_variables::text, 1, 150) as preview
   FROM workspace_tasks 
   WHERE workspace_id = 'ws-mb7m9ii5ey' 
   ORDER BY id DESC LIMIT 1;"

echo ""
echo "4. 预期结果:"
echo "   - 长度应该约为 200-300 字节（不是 1500+ 字节）"
echo "   - 预览应该只包含 4 个字段：workspace_id, variable_id, version, variable_type"
echo "   - 不应该有 id, key, created_at 等字段"
