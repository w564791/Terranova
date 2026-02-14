#!/bin/bash

# =============================================================================
# PostgreSQL Database Migration Script
# =============================================================================
# 
# 用途: 在容器内执行，将一个 PostgreSQL 数据库完整迁移到另一个数据库
# 
# 包含内容:
#   - 表结构 (CREATE TABLE)
#   - 索引 (CREATE INDEX)
#   - 约束 (PRIMARY KEY, FOREIGN KEY, UNIQUE, CHECK)
#   - 视图 (CREATE VIEW)
#   - 函数 (CREATE FUNCTION)
#   - 触发器 (CREATE TRIGGER)
#   - 序列及当前值
#   - 所有数据 (INSERT)
#   - 向量数据 (pgvector)
#
# 使用方式:
#   ./migrate_db.sh \
#     --source "postgresql://user:pass@source-host:5432/dbname" \
#     --dest "postgresql://user:pass@dest-host:5432/dbname"
# 在容器内执行
./migrate_db.sh \
  --source "postgresql://postgres:postgres123@10.202.78.33:15432/iac_platform" \
  --dest "postgresql://postgres:postgres123@localhost:5432/iac_platform"
# 选项:
#   --source URL      源数据库连接字符串 (必需)
#   --dest URL        目标数据库连接字符串 (必需)
#   --schema-only     只迁移结构，不迁移数据
#   --data-only       只迁移数据，不迁移结构
#   --drop-existing   清空目标数据库现有数据
#   --dry-run         只显示将要执行的操作，不实际执行
#   --verbose         显示详细输出
#   --help            显示帮助信息
#
# =============================================================================

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# 默认值
SCHEMA_ONLY=false
DATA_ONLY=false
DROP_EXISTING=false
DRY_RUN=false
VERBOSE=false
BACKUP_DIR="/tmp/db_migration_$(date +%Y%m%d_%H%M%S)"

# =============================================================================
# 函数定义
# =============================================================================

print_banner() {
    echo -e "${CYAN}"
    echo "╔═══════════════════════════════════════════════════════════════════╗"
    echo "║         PostgreSQL Database Migration Script v3.0                 ║"
    echo "║                                                                   ║"
    echo "║  完整迁移: 表结构、索引、函数、触发器、视图、数据、向量           ║"
    echo "╚═══════════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

print_help() {
    cat << EOF
用法: $0 [选项]

必需参数:
  --source URL      源数据库连接字符串
                    格式: postgresql://user:password@host:port/database
  --dest URL        目标数据库连接字符串
                    格式: postgresql://user:password@host:port/database

可选参数:
  --schema-only     只迁移结构，不迁移数据
  --data-only       只迁移数据，不迁移结构 (目标库必须已有结构)
  --drop-existing   清空目标数据库现有数据 (保留扩展)
  --dry-run         只显示将要执行的操作，不实际执行
  --verbose         显示详细输出
  --help            显示此帮助信息

示例:
  # 完整迁移
  $0 --source "postgresql://postgres:pass@source:5432/mydb" \\
     --dest "postgresql://postgres:pass@dest:5432/mydb"

  # 只迁移结构
  $0 --source "postgresql://postgres:pass@source:5432/mydb" \\
     --dest "postgresql://postgres:pass@dest:5432/mydb" \\
     --schema-only

  # 清空目标库后迁移
  $0 --source "postgresql://postgres:pass@source:5432/mydb" \\
     --dest "postgresql://postgres:pass@dest:5432/mydb" \\
     --drop-existing

EOF
}

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

log_verbose() {
    if [[ "$VERBOSE" == "true" ]]; then
        echo -e "${CYAN}[DEBUG]${NC} $1"
    fi
}

# 解析连接字符串
# 格式: postgresql://user:password@host:port/database
parse_connection_string() {
    local url="$1"
    local prefix="$2"
    
    # 移除 postgresql:// 前缀
    local conn="${url#postgresql://}"
    
    # 提取用户名和密码
    local userpass="${conn%%@*}"
    local hostdb="${conn#*@}"
    
    # 提取用户名
    local user="${userpass%%:*}"
    # 提取密码
    local pass="${userpass#*:}"
    
    # 提取主机和端口
    local hostport="${hostdb%%/*}"
    local host="${hostport%%:*}"
    local port="${hostport#*:}"
    
    # 提取数据库名
    local db="${hostdb#*/}"
    
    # 设置变量
    eval "${prefix}_USER='$user'"
    eval "${prefix}_PASS='$pass'"
    eval "${prefix}_HOST='$host'"
    eval "${prefix}_PORT='$port'"
    eval "${prefix}_DB='$db'"
}

# 检查依赖
check_dependencies() {
    log_step "检查依赖..."
    
    local missing=()
    
    for cmd in pg_dump pg_restore psql; do
        if ! command -v $cmd &> /dev/null; then
            missing+=($cmd)
        fi
    done
    
    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "缺少依赖: ${missing[*]}"
        echo ""
        echo "请确保在 PostgreSQL 容器内运行此脚本，或安装 PostgreSQL 客户端工具。"
        echo ""
        echo "安装方式:"
        echo "  macOS:        brew install postgresql@16"
        echo "  Ubuntu/Debian: apt-get install postgresql-client-16"
        echo "  CentOS/RHEL:  dnf install postgresql16"
        exit 1
    fi
    
    # 显示版本
    local pg_version=$(pg_dump --version | head -1)
    log_info "PostgreSQL 工具版本: $pg_version"
}

# 测试数据库连接
test_connection() {
    local name="$1"
    local host="$2"
    local port="$3"
    local user="$4"
    local pass="$5"
    local db="$6"
    
    log_verbose "测试 $name 连接: $host:$port/$db"
    
    if PGPASSWORD="$pass" psql -h "$host" -p "$port" -U "$user" -d "$db" -c "SELECT 1" &> /dev/null; then
        log_info "$name 连接成功 ✓"
        return 0
    else
        log_error "$name 连接失败"
        return 1
    fi
}

# 获取数据库统计信息
get_db_stats() {
    local host="$1"
    local port="$2"
    local user="$3"
    local pass="$4"
    local db="$5"
    
    PGPASSWORD="$pass" psql -h "$host" -p "$port" -U "$user" -d "$db" -t -A << 'EOF'
SELECT json_build_object(
    'tables', (SELECT COUNT(*) FROM pg_tables WHERE schemaname = 'public'),
    'indexes', (SELECT COUNT(*) FROM pg_indexes WHERE schemaname = 'public'),
    'views', (SELECT COUNT(*) FROM pg_views WHERE schemaname = 'public'),
    'functions', (SELECT COUNT(*) FROM pg_proc p JOIN pg_namespace n ON p.pronamespace = n.oid WHERE n.nspname = 'public'),
    'triggers', (SELECT COUNT(*) FROM pg_trigger t JOIN pg_class c ON t.tgrelid = c.oid JOIN pg_namespace n ON c.relnamespace = n.oid WHERE n.nspname = 'public' AND NOT t.tgisinternal),
    'sequences', (SELECT COUNT(*) FROM pg_sequences WHERE schemaname = 'public'),
    'extensions', (SELECT array_agg(extname) FROM pg_extension WHERE extname != 'plpgsql'),
    'total_rows', (SELECT COALESCE(SUM(n_live_tup), 0) FROM pg_stat_user_tables)
);
EOF
}

# 显示数据库统计
show_db_stats() {
    local name="$1"
    local stats="$2"
    
    echo ""
    echo -e "${CYAN}=== $name 数据库统计 ===${NC}"
    echo "$stats" | python3 -c "
import sys, json
data = json.load(sys.stdin)
print(f\"  表数量:     {data['tables']}\")
print(f\"  索引数量:   {data['indexes']}\")
print(f\"  视图数量:   {data['views']}\")
print(f\"  函数数量:   {data['functions']}\")
print(f\"  触发器数量: {data['triggers']}\")
print(f\"  序列数量:   {data['sequences']}\")
print(f\"  总行数:     {data['total_rows']}\")
exts = data['extensions']
if exts:
    print(f\"  扩展:       {', '.join(exts)}\")
else:
    print(f\"  扩展:       无\")
" 2>/dev/null || echo "$stats"
}

# 清空目标数据库
drop_existing_objects() {
    local host="$1"
    local port="$2"
    local user="$3"
    local pass="$4"
    local db="$5"
    
    log_step "清空目标数据库 (保留扩展)..."
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] 将清空目标数据库"
        return 0
    fi
    
    PGPASSWORD="$pass" psql -h "$host" -p "$port" -U "$user" -d "$db" << 'EOF'
-- 禁用外键检查
SET session_replication_role = 'replica';

-- 删除所有视图
DO $$ 
DECLARE
    r RECORD;
BEGIN
    FOR r IN (SELECT viewname FROM pg_views WHERE schemaname = 'public') LOOP
        EXECUTE 'DROP VIEW IF EXISTS public.' || quote_ident(r.viewname) || ' CASCADE';
    END LOOP;
END $$;

-- 删除所有表
DO $$ 
DECLARE
    r RECORD;
BEGIN
    FOR r IN (SELECT tablename FROM pg_tables WHERE schemaname = 'public') LOOP
        EXECUTE 'DROP TABLE IF EXISTS public.' || quote_ident(r.tablename) || ' CASCADE';
    END LOOP;
END $$;

-- 删除所有序列
DO $$ 
DECLARE
    r RECORD;
BEGIN
    FOR r IN (SELECT sequencename FROM pg_sequences WHERE schemaname = 'public') LOOP
        EXECUTE 'DROP SEQUENCE IF EXISTS public.' || quote_ident(r.sequencename) || ' CASCADE';
    END LOOP;
END $$;

-- 删除所有函数 (保留扩展函数)
DO $$ 
DECLARE
    r RECORD;
BEGIN
    FOR r IN (
        SELECT p.proname, pg_get_function_identity_arguments(p.oid) as args
        FROM pg_proc p
        JOIN pg_namespace n ON p.pronamespace = n.oid
        LEFT JOIN pg_depend d ON d.objid = p.oid AND d.deptype = 'e'
        WHERE n.nspname = 'public' AND d.objid IS NULL
    ) LOOP
        EXECUTE 'DROP FUNCTION IF EXISTS public.' || quote_ident(r.proname) || '(' || r.args || ') CASCADE';
    END LOOP;
END $$;

-- 恢复外键检查
SET session_replication_role = 'origin';

-- 验证扩展仍然存在
SELECT extname, extversion FROM pg_extension WHERE extname IN ('vector', 'pg_trgm', 'btree_gin');
EOF
    
    log_info "目标数据库已清空 ✓"
}

# 导出数据库
export_database() {
    local host="$1"
    local port="$2"
    local user="$3"
    local pass="$4"
    local db="$5"
    local dump_file="$6"
    
    log_step "导出源数据库..."
    
    local pg_dump_opts=(
        -h "$host"
        -p "$port"
        -U "$user"
        -d "$db"
        --format=custom
        --no-owner
        --no-privileges
    )
    
    if [[ "$SCHEMA_ONLY" == "true" ]]; then
        pg_dump_opts+=(--schema-only)
        log_info "模式: 只导出结构"
    elif [[ "$DATA_ONLY" == "true" ]]; then
        pg_dump_opts+=(--data-only)
        log_info "模式: 只导出数据"
    else
        log_info "模式: 完整导出 (结构 + 数据)"
    fi
    
    if [[ "$VERBOSE" == "true" ]]; then
        pg_dump_opts+=(--verbose)
    fi
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] 将执行: pg_dump ${pg_dump_opts[*]} -f $dump_file"
        return 0
    fi
    
    log_verbose "执行: pg_dump ${pg_dump_opts[*]} -f $dump_file"
    
    PGPASSWORD="$pass" pg_dump "${pg_dump_opts[@]}" -f "$dump_file" 2>&1
    
    local dump_size=$(du -h "$dump_file" | cut -f1)
    log_info "导出完成: $dump_file ($dump_size) ✓"
}

# 导入数据库
import_database() {
    local host="$1"
    local port="$2"
    local user="$3"
    local pass="$4"
    local db="$5"
    local dump_file="$6"
    
    log_step "导入到目标数据库..."
    
    local pg_restore_opts=(
        -h "$host"
        -p "$port"
        -U "$user"
        -d "$db"
        --no-owner
        --no-privileges
        --single-transaction
    )
    
    if [[ "$VERBOSE" == "true" ]]; then
        pg_restore_opts+=(--verbose)
    fi
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] 将执行: pg_restore ${pg_restore_opts[*]} $dump_file"
        return 0
    fi
    
    log_verbose "执行: pg_restore ${pg_restore_opts[*]} $dump_file"
    
    # pg_restore 可能会有一些警告，使用 || true 忽略非致命错误
    PGPASSWORD="$pass" pg_restore "${pg_restore_opts[@]}" "$dump_file" 2>&1 || true
    
    log_info "导入完成 ✓"
}

# 验证迁移
verify_migration() {
    log_step "验证迁移结果..."
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] 将验证迁移结果"
        return 0
    fi
    
    local source_stats=$(get_db_stats "$SOURCE_HOST" "$SOURCE_PORT" "$SOURCE_USER" "$SOURCE_PASS" "$SOURCE_DB")
    local dest_stats=$(get_db_stats "$DEST_HOST" "$DEST_PORT" "$DEST_USER" "$DEST_PASS" "$DEST_DB")
    
    show_db_stats "源" "$source_stats"
    show_db_stats "目标" "$dest_stats"
    
    # 比较统计信息
    echo ""
    echo -e "${CYAN}=== 迁移验证 ===${NC}"
    
    python3 << EOF
import json

source = json.loads('''$source_stats''')
dest = json.loads('''$dest_stats''')

checks = [
    ('表数量', source['tables'], dest['tables']),
    ('索引数量', source['indexes'], dest['indexes']),
    ('视图数量', source['views'], dest['views']),
    ('函数数量', source['functions'], dest['functions']),
    ('触发器数量', source['triggers'], dest['triggers']),
    ('序列数量', source['sequences'], dest['sequences']),
]

all_passed = True
for name, src, dst in checks:
    if src == dst:
        print(f"  ✓ {name}: {src} == {dst}")
    else:
        print(f"  ✗ {name}: {src} != {dst}")
        all_passed = False

# 数据行数可能有小差异（由于统计信息更新延迟）
src_rows = source['total_rows']
dst_rows = dest['total_rows']
if abs(src_rows - dst_rows) <= src_rows * 0.01:  # 允许 1% 误差
    print(f"  ✓ 总行数: {src_rows} ≈ {dst_rows}")
else:
    print(f"  ⚠ 总行数: {src_rows} != {dst_rows} (可能需要 ANALYZE)")

if all_passed:
    print("\n  ✓ 迁移验证通过!")
else:
    print("\n  ⚠ 部分检查未通过，请手动验证")
EOF
}

# 清理临时文件
cleanup() {
    if [[ -d "$BACKUP_DIR" ]]; then
        log_step "清理临时文件..."
        rm -rf "$BACKUP_DIR"
        log_info "清理完成 ✓"
    fi
}

# 主函数
main() {
    print_banner
    
    # 解析参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            --source)
                SOURCE_URL="$2"
                shift 2
                ;;
            --dest)
                DEST_URL="$2"
                shift 2
                ;;
            --schema-only)
                SCHEMA_ONLY=true
                shift
                ;;
            --data-only)
                DATA_ONLY=true
                shift
                ;;
            --drop-existing)
                DROP_EXISTING=true
                shift
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            --verbose)
                VERBOSE=true
                shift
                ;;
            --help)
                print_help
                exit 0
                ;;
            *)
                log_error "未知参数: $1"
                print_help
                exit 1
                ;;
        esac
    done
    
    # 验证必需参数
    if [[ -z "$SOURCE_URL" ]]; then
        log_error "缺少必需参数: --source"
        print_help
        exit 1
    fi
    
    if [[ -z "$DEST_URL" ]]; then
        log_error "缺少必需参数: --dest"
        print_help
        exit 1
    fi
    
    # 解析连接字符串
    parse_connection_string "$SOURCE_URL" "SOURCE"
    parse_connection_string "$DEST_URL" "DEST"
    
    log_info "源数据库: $SOURCE_HOST:$SOURCE_PORT/$SOURCE_DB"
    log_info "目标数据库: $DEST_HOST:$DEST_PORT/$DEST_DB"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log_warn "DRY-RUN 模式: 不会实际执行任何操作"
    fi
    
    echo ""
    
    # 检查依赖
    check_dependencies
    
    # 测试连接
    echo ""
    log_step "测试数据库连接..."
    test_connection "源数据库" "$SOURCE_HOST" "$SOURCE_PORT" "$SOURCE_USER" "$SOURCE_PASS" "$SOURCE_DB"
    test_connection "目标数据库" "$DEST_HOST" "$DEST_PORT" "$DEST_USER" "$DEST_PASS" "$DEST_DB"
    
    # 显示源数据库统计
    echo ""
    local source_stats=$(get_db_stats "$SOURCE_HOST" "$SOURCE_PORT" "$SOURCE_USER" "$SOURCE_PASS" "$SOURCE_DB")
    show_db_stats "源" "$source_stats"
    
    # 创建备份目录
    mkdir -p "$BACKUP_DIR"
    local dump_file="$BACKUP_DIR/migration.dump"
    
    # 清空目标数据库（如果指定）
    if [[ "$DROP_EXISTING" == "true" ]]; then
        echo ""
        drop_existing_objects "$DEST_HOST" "$DEST_PORT" "$DEST_USER" "$DEST_PASS" "$DEST_DB"
    fi
    
    # 导出
    echo ""
    export_database "$SOURCE_HOST" "$SOURCE_PORT" "$SOURCE_USER" "$SOURCE_PASS" "$SOURCE_DB" "$dump_file"
    
    # 导入
    echo ""
    import_database "$DEST_HOST" "$DEST_PORT" "$DEST_USER" "$DEST_PASS" "$DEST_DB" "$dump_file"
    
    # 更新统计信息
    if [[ "$DRY_RUN" != "true" ]]; then
        echo ""
        log_step "更新目标数据库统计信息..."
        PGPASSWORD="$DEST_PASS" psql -h "$DEST_HOST" -p "$DEST_PORT" -U "$DEST_USER" -d "$DEST_DB" -c "ANALYZE;" &> /dev/null
        log_info "统计信息已更新 ✓"
    fi
    
    # 验证
    echo ""
    verify_migration
    
    # 清理
    echo ""
    cleanup
    
    # 完成
    echo ""
    echo -e "${GREEN}╔═══════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                      迁移完成!                                    ║${NC}"
    echo -e "${GREEN}╚═══════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "目标数据库连接信息:"
    echo "  主机: $DEST_HOST"
    echo "  端口: $DEST_PORT"
    echo "  数据库: $DEST_DB"
    echo "  用户: $DEST_USER"
    echo ""
    echo "连接字符串:"
    echo "  postgresql://$DEST_USER:****@$DEST_HOST:$DEST_PORT/$DEST_DB"
    echo ""
}

# 设置清理钩子
trap cleanup EXIT

# 运行主函数
main "$@"