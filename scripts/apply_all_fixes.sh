#!/bin/bash
# 一次性应用所有数据库修复
# 使用环境变量避免输入密码

export PGPASSWORD=postgres123

echo "正在修复workspaces表..."
psql -h localhost -U postgres -d iac_platform -f scripts/fix_workspaces_columns.sql

echo ""
echo "正在修复workspace_tasks表..."
psql -h localhost -U postgres -d iac_platform -f scripts/fix_workspace_tasks_all_columns.sql

echo ""
echo " 所有数据库修复已完成！"
echo "请重启后端服务器以加载新的Schema"
