#!/bin/bash

set -e

echo "修复 user_handler.go 中的 user_id 解析..."

# 备份文件
cp backend/internal/handlers/user_handler.go backend/internal/handlers/user_handler.go.bak

# 修复所有 ParseUint 调用 - 直接使用 idStr 作为 user_id
# 将:
#   idStr := c.Param("id")
#   id, err := strconv.ParseUint(idStr, 10, 32)
#   if err != nil { ... }
#   ... uint(id) ...
# 改为:
#   userID := c.Param("id")
#   ... userID ...

perl -i -pe '
    # 将 idStr := c.Param("id") 改为 userID := c.Param("id")
    s/idStr := c\.Param\("id"\)/userID := c.Param("id")/g;
    
    # 删除 ParseUint 相关的行(通过标记删除)
    if (/id, err := strconv\.ParseUint\(idStr, 10, 32\)/) {
        $_ = "";
        $delete_next = 3;  # 删除接下来的3行(err check)
    }
    if ($delete_next > 0) {
        $_ = "";
        $delete_next--;
    }
    
    # 将 uint(id) 改为 userID
    s/uint\(id\)/userID/g;
' backend/internal/handlers/user_handler.go

echo " user_handler.go 已修复"
echo "备份文件: backend/internal/handlers/user_handler.go.bak"
