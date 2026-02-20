#!/bin/bash

# 代码行数统计脚本
# 统计文档、Go代码、前端代码的行数

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# 获取脚本所在目录的父目录（项目根目录）
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_ROOT"

echo -e "${CYAN}========================================${NC}"
echo -e "${CYAN}       代码行数统计报告${NC}"
echo -e "${CYAN}========================================${NC}"
echo -e "项目路径: ${PROJECT_ROOT}"
echo -e "统计时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

# 函数：统计指定类型文件的行数
count_lines() {
    local pattern="$1"
    local exclude_dirs="$2"
    local files
    
    if [ -n "$exclude_dirs" ]; then
        files=$(find . -type f -name "$pattern" $exclude_dirs 2>/dev/null)
    else
        files=$(find . -type f -name "$pattern" 2>/dev/null)
    fi
    
    if [ -z "$files" ]; then
        echo "0"
        return
    fi
    
    echo "$files" | xargs wc -l 2>/dev/null | tail -1 | awk '{print $1}'
}

# 函数：统计多个模式的文件行数
count_lines_multi() {
    local patterns="$1"
    local exclude_dirs="$2"
    local total=0
    
    for pattern in $patterns; do
        local count=$(count_lines "$pattern" "$exclude_dirs")
        total=$((total + count))
    done
    
    echo "$total"
}

# 函数：统计文件数量
count_files() {
    local pattern="$1"
    local exclude_dirs="$2"
    
    if [ -n "$exclude_dirs" ]; then
        find . -type f -name "$pattern" $exclude_dirs 2>/dev/null | wc -l | tr -d ' '
    else
        find . -type f -name "$pattern" 2>/dev/null | wc -l | tr -d ' '
    fi
}

# 排除目录
EXCLUDE_DIRS="-not -path '*/node_modules/*' -not -path '*/.git/*' -not -path '*/vendor/*' -not -path '*/dist/*' -not -path '*/build/*'"

echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}📚 文档统计 (Markdown)${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

# 统计 Markdown 文件
md_files=$(find . -type f -name "*.md" -not -path '*/node_modules/*' -not -path '*/.git/*' 2>/dev/null | wc -l | tr -d ' ')
md_lines=$(find . -type f -name "*.md" -not -path '*/node_modules/*' -not -path '*/.git/*' 2>/dev/null | xargs wc -l 2>/dev/null | tail -1 | awk '{print $1}')
md_lines=${md_lines:-0}

echo -e "  文件数量: ${BLUE}${md_files}${NC} 个"
echo -e "  总行数:   ${BLUE}${md_lines}${NC} 行"

# 按目录统计 Markdown
echo -e "\n  ${CYAN}按目录分布:${NC}"
for dir in docs backend/docs frontend; do
    if [ -d "$dir" ]; then
        dir_md_files=$(find "$dir" -type f -name "*.md" 2>/dev/null | wc -l | tr -d ' ')
        dir_md_lines=$(find "$dir" -type f -name "*.md" 2>/dev/null | xargs wc -l 2>/dev/null | tail -1 | awk '{print $1}')
        dir_md_lines=${dir_md_lines:-0}
        if [ "$dir_md_files" -gt 0 ]; then
            printf "    %-20s %6s 文件, %8s 行\n" "$dir/" "$dir_md_files" "$dir_md_lines"
        fi
    fi
done

echo ""
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}🔧 Go 代码统计${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

# 统计 Go 文件
go_files=$(find . -type f -name "*.go" -not -path '*/vendor/*' -not -path '*/.git/*' 2>/dev/null | wc -l | tr -d ' ')
go_lines=$(find . -type f -name "*.go" -not -path '*/vendor/*' -not -path '*/.git/*' 2>/dev/null | xargs wc -l 2>/dev/null | tail -1 | awk '{print $1}')
go_lines=${go_lines:-0}

echo -e "  文件数量: ${BLUE}${go_files}${NC} 个"
echo -e "  总行数:   ${BLUE}${go_lines}${NC} 行"

# 按目录统计 Go
echo -e "\n  ${CYAN}按目录分布:${NC}"
for dir in backend agent demo; do
    if [ -d "$dir" ]; then
        dir_go_files=$(find "$dir" -type f -name "*.go" -not -path '*/vendor/*' 2>/dev/null | wc -l | tr -d ' ')
        dir_go_lines=$(find "$dir" -type f -name "*.go" -not -path '*/vendor/*' 2>/dev/null | xargs wc -l 2>/dev/null | tail -1 | awk '{print $1}')
        dir_go_lines=${dir_go_lines:-0}
        if [ "$dir_go_files" -gt 0 ]; then
            printf "    %-20s %6s 文件, %8s 行\n" "$dir/" "$dir_go_files" "$dir_go_lines"
        fi
    fi
done

# 统计测试文件
go_test_files=$(find . -type f -name "*_test.go" -not -path '*/vendor/*' -not -path '*/.git/*' 2>/dev/null | wc -l | tr -d ' ')
go_test_lines=$(find . -type f -name "*_test.go" -not -path '*/vendor/*' -not -path '*/.git/*' 2>/dev/null | xargs wc -l 2>/dev/null | tail -1 | awk '{print $1}')
go_test_lines=${go_test_lines:-0}

echo -e "\n  ${CYAN}测试代码:${NC}"
echo -e "    测试文件: ${BLUE}${go_test_files}${NC} 个"
echo -e "    测试行数: ${BLUE}${go_test_lines}${NC} 行"

echo ""
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}🎨 前端代码统计${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

# 统计 TypeScript 文件
ts_files=$(find ./frontend -type f \( -name "*.ts" -o -name "*.tsx" \) -not -path '*/node_modules/*' -not -path '*/dist/*' 2>/dev/null | wc -l | tr -d ' ')
ts_lines=$(find ./frontend -type f \( -name "*.ts" -o -name "*.tsx" \) -not -path '*/node_modules/*' -not -path '*/dist/*' 2>/dev/null | xargs wc -l 2>/dev/null | tail -1 | awk '{print $1}')
ts_lines=${ts_lines:-0}

# 统计 JavaScript 文件
js_files=$(find ./frontend -type f \( -name "*.js" -o -name "*.jsx" \) -not -path '*/node_modules/*' -not -path '*/dist/*' 2>/dev/null | wc -l | tr -d ' ')
js_lines=$(find ./frontend -type f \( -name "*.js" -o -name "*.jsx" \) -not -path '*/node_modules/*' -not -path '*/dist/*' 2>/dev/null | xargs wc -l 2>/dev/null | tail -1 | awk '{print $1}')
js_lines=${js_lines:-0}

# 统计 CSS 文件
css_files=$(find ./frontend -type f \( -name "*.css" -o -name "*.scss" -o -name "*.less" \) -not -path '*/node_modules/*' -not -path '*/dist/*' 2>/dev/null | wc -l | tr -d ' ')
css_lines=$(find ./frontend -type f \( -name "*.css" -o -name "*.scss" -o -name "*.less" \) -not -path '*/node_modules/*' -not -path '*/dist/*' 2>/dev/null | xargs wc -l 2>/dev/null | tail -1 | awk '{print $1}')
css_lines=${css_lines:-0}

# 统计 HTML 文件
html_files=$(find ./frontend -type f -name "*.html" -not -path '*/node_modules/*' -not -path '*/dist/*' 2>/dev/null | wc -l | tr -d ' ')
html_lines=$(find ./frontend -type f -name "*.html" -not -path '*/node_modules/*' -not -path '*/dist/*' 2>/dev/null | xargs wc -l 2>/dev/null | tail -1 | awk '{print $1}')
html_lines=${html_lines:-0}

# 统计 JSON 配置文件
json_files=$(find ./frontend -type f -name "*.json" -not -path '*/node_modules/*' -not -path '*/dist/*' 2>/dev/null | wc -l | tr -d ' ')
json_lines=$(find ./frontend -type f -name "*.json" -not -path '*/node_modules/*' -not -path '*/dist/*' 2>/dev/null | xargs wc -l 2>/dev/null | tail -1 | awk '{print $1}')
json_lines=${json_lines:-0}

# 前端总计
frontend_files=$((ts_files + js_files + css_files + html_files))
frontend_lines=$((ts_lines + js_lines + css_lines + html_lines))

echo -e "  ${CYAN}TypeScript (.ts/.tsx):${NC}"
echo -e "    文件数量: ${BLUE}${ts_files}${NC} 个"
echo -e "    总行数:   ${BLUE}${ts_lines}${NC} 行"

echo -e "\n  ${CYAN}JavaScript (.js/.jsx):${NC}"
echo -e "    文件数量: ${BLUE}${js_files}${NC} 个"
echo -e "    总行数:   ${BLUE}${js_lines}${NC} 行"

echo -e "\n  ${CYAN}样式文件 (.css/.scss/.less):${NC}"
echo -e "    文件数量: ${BLUE}${css_files}${NC} 个"
echo -e "    总行数:   ${BLUE}${css_lines}${NC} 行"

echo -e "\n  ${CYAN}HTML 文件:${NC}"
echo -e "    文件数量: ${BLUE}${html_files}${NC} 个"
echo -e "    总行数:   ${BLUE}${html_lines}${NC} 行"

echo -e "\n  ${CYAN}前端代码总计:${NC}"
echo -e "    文件数量: ${BLUE}${frontend_files}${NC} 个"
echo -e "    总行数:   ${BLUE}${frontend_lines}${NC} 行"

# 按子目录统计前端代码
echo -e "\n  ${CYAN}按子目录分布:${NC}"
for subdir in src/pages src/components src/services src/hooks src/utils src/contexts; do
    if [ -d "./frontend/$subdir" ]; then
        subdir_files=$(find "./frontend/$subdir" -type f \( -name "*.ts" -o -name "*.tsx" -o -name "*.js" -o -name "*.jsx" -o -name "*.css" \) 2>/dev/null | wc -l | tr -d ' ')
        subdir_lines=$(find "./frontend/$subdir" -type f \( -name "*.ts" -o -name "*.tsx" -o -name "*.js" -o -name "*.jsx" -o -name "*.css" \) 2>/dev/null | xargs wc -l 2>/dev/null | tail -1 | awk '{print $1}')
        subdir_lines=${subdir_lines:-0}
        if [ "$subdir_files" -gt 0 ]; then
            printf "    %-25s %6s 文件, %8s 行\n" "frontend/$subdir/" "$subdir_files" "$subdir_lines"
        fi
    fi
done

echo ""
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}📊 SQL 脚本统计${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

sql_files=$(find . -type f -name "*.sql" -not -path '*/node_modules/*' -not -path '*/.git/*' 2>/dev/null | wc -l | tr -d ' ')
sql_lines=$(find . -type f -name "*.sql" -not -path '*/node_modules/*' -not -path '*/.git/*' 2>/dev/null | xargs wc -l 2>/dev/null | tail -1 | awk '{print $1}')
sql_lines=${sql_lines:-0}

echo -e "  文件数量: ${BLUE}${sql_files}${NC} 个"
echo -e "  总行数:   ${BLUE}${sql_lines}${NC} 行"

echo ""
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}📈 总计${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

total_files=$((md_files + go_files + frontend_files + sql_files))
total_lines=$((md_lines + go_lines + frontend_lines + sql_lines))

echo ""
printf "  ${CYAN}%-25s${NC} %8s 文件  %10s 行\n" "文档 (Markdown)" "$md_files" "$md_lines"
printf "  ${CYAN}%-25s${NC} %8s 文件  %10s 行\n" "Go 代码" "$go_files" "$go_lines"
printf "  ${CYAN}%-25s${NC} %8s 文件  %10s 行\n" "前端代码" "$frontend_files" "$frontend_lines"
printf "  ${CYAN}%-25s${NC} %8s 文件  %10s 行\n" "SQL 脚本" "$sql_files" "$sql_lines"
echo -e "  ${YELLOW}─────────────────────────────────────────────────${NC}"
printf "  ${GREEN}%-25s${NC} %8s 文件  %10s 行\n" "总计" "$total_files" "$total_lines"

echo ""
echo -e "${CYAN}========================================${NC}"
echo -e "${GREEN}统计完成！${NC}"
echo -e "${CYAN}========================================${NC}"
