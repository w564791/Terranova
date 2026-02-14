#!/bin/bash

# 手动触发任务601的执行
# 这个脚本会通过API触发任务队列管理器重新尝试执行任务

echo "=== 触发任务601执行 ==="
echo ""

# 方法1: 直接调用后端API (需要认证token)
echo "请在浏览器中执行以下操作:"
echo "1. 打开浏览器开发者工具 (F12)"
echo "2. 在Console中执行以下代码:"
echo ""
echo "fetch('http://localhost:8080/api/v1/workspaces/ws-mb7m9ii5ey/tasks/601/confirm-apply', {"
echo "  method: 'POST',"
echo "  headers: {"
echo "    'Content-Type': 'application/json',"
echo "    'Authorization': 'Bearer ' + localStorage.getItem('token')"
echo "  },"
echo "  body: JSON.stringify({"
echo "    apply_description: 'Manual retry after Pod deletion'"
echo "  })"
echo "}).then(r => r.json()).then(console.log)"
echo ""
echo "=== 或者 ==="
echo ""
echo "直接在任务详情页面重新点击 'Confirm & Apply' 按钮"
