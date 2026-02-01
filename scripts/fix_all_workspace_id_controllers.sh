#!/bin/bash

# 批量修复所有控制器文件中的 workspace ID 解析
# 将 ParseUint(ctx.Param("id") 改为支持语义化ID的查询

cd /Users/ken/go/src/iac-platform/backend/controllers

echo "开始修复控制器文件..."

# 需要修复的文件列表
files=(
  "resource_controller.go"
  "workspace_variable_controller.go"
  "state_version_controller.go"
)

for file in "${files[@]}"; do
  if [ -f "$file" ]; then
    echo "修复 $file..."
    
    # 备份
    cp "$file" "${file}.backup"
    
    # 创建临时 Go 文件来进行智能替换
    cat > /tmp/fix_${file}.go << 'GOEOF'
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	file := os.Args[1]
	input, _ := os.Open(file)
	defer input.Close()
	
	var lines []string
	scanner := bufio.NewScanner(input)
	
	for scanner.Scan() {
		line := scanner.Text()
		
		// 替换 workspaceID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
		if strings.Contains(line, `workspaceID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)`) {
			lines = append(lines, strings.Replace(line, 
				`workspaceID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)`,
				`workspaceIDParam := ctx.Param("id")`, 1))
			continue
		}
		
		// 下一行的 if err != nil 改为 if workspaceIDParam == ""
		if strings.Contains(line, "if err != nil {") && len(lines) > 0 && 
		   strings.Contains(lines[len(lines)-1], "workspaceIDParam") {
			lines = append(lines, strings.Replace(line, "if err != nil {", "if workspaceIDParam == \"\" {", 1))
			continue
		}
		
		lines = append(lines, line)
	}
	
	// 写回文件
	output, _ := os.Create(file)
	defer output.Close()
	
	for _, line := range lines {
		fmt.Fprintln(output, line)
	}
}
GOEOF
    
    # 简单的 sed 替换
    sed -i.tmp '
      # 第一步：替换 ParseUint 为直接获取参数
      s/workspaceID, err := strconv\.ParseUint(ctx\.Param("id"), 10, 32)/workspaceIDParam := ctx.Param("id")/g
      s/workspaceID, _ := strconv\.ParseUint(ctx\.Param("id"), 10, 32)/workspaceIDParam := ctx.Param("id")/g
    ' "$file"
    
    # 第二步：在获取参数后添加 workspace 查询逻辑
    # 这需要手动处理，因为太复杂了
    
    echo "  ✓ $file 基础替换完成（需要手动添加 workspace 查询逻辑）"
  else
    echo "  ✗ $file 不存在"
  fi
done

echo ""
echo "修复完成！"
echo "注意：这些文件需要手动添加以下代码："
echo ""
echo "  // 获取workspace以获取内部ID"
echo "  var workspace models.Workspace"
echo "  err := c.db.Where(\"workspace_id = ?\", workspaceIDParam).First(&workspace).Error"
echo "  if err != nil {"
echo "    if err := c.db.Where(\"id = ?\", workspaceIDParam).First(&workspace).Error; err != nil {"
echo "      ctx.JSON(http.StatusNotFound, gin.H{\"error\": \"Workspace not found\"})"
echo "      return"
echo "    }"
echo "  }"
echo "  workspaceID := workspace.ID"
echo ""
echo "请手动完成剩余修改，或使用 replace_in_file 工具。"
