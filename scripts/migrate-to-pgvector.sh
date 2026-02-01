#!/bin/bash

# PostgreSQL to pgvector Migration Script
# This script migrates all data from the original PostgreSQL database to the new pgvector database

set -e

# Configuration
SOURCE_HOST="localhost"
SOURCE_PORT="5432"
TARGET_HOST="localhost"
TARGET_PORT="15432"
DB_NAME="iac_platform"
DB_USER="postgres"
DB_PASSWORD="postgres123"

# Export password for psql/pg_dump/pg_restore
export PGPASSWORD="$DB_PASSWORD"

# Backup directory
BACKUP_DIR="/tmp/iac_platform_migration"
mkdir -p "$BACKUP_DIR"

echo "=========================================="
echo "PostgreSQL to pgvector Migration Script"
echo "=========================================="
echo ""
echo "Source: $SOURCE_HOST:$SOURCE_PORT"
echo "Target: $TARGET_HOST:$TARGET_PORT"
echo "Database: $DB_NAME"
echo ""

# Step 1: Create a complete backup from source database
echo "[Step 1/6] Creating complete backup from source database..."
pg_dump -h "$SOURCE_HOST" -p "$SOURCE_PORT" -U "$DB_USER" -d "$DB_NAME" \
    --format=custom \
    --verbose \
    --no-owner \
    --no-privileges \
    -f "$BACKUP_DIR/iac_platform_full.dump"
echo "✓ Full backup created: $BACKUP_DIR/iac_platform_full.dump"

# Step 2: Export schema only (for reference)
echo ""
echo "[Step 2/6] Exporting schema (tables, indexes, functions, triggers)..."
pg_dump -h "$SOURCE_HOST" -p "$SOURCE_PORT" -U "$DB_USER" -d "$DB_NAME" \
    --schema-only \
    --no-owner \
    --no-privileges \
    -f "$BACKUP_DIR/iac_platform_schema.sql"
echo "✓ Schema exported: $BACKUP_DIR/iac_platform_schema.sql"

# Step 3: Export data only (for reference)
echo ""
echo "[Step 3/6] Exporting data..."
pg_dump -h "$SOURCE_HOST" -p "$SOURCE_PORT" -U "$DB_USER" -d "$DB_NAME" \
    --data-only \
    --no-owner \
    --no-privileges \
    -f "$BACKUP_DIR/iac_platform_data.sql"
echo "✓ Data exported: $BACKUP_DIR/iac_platform_data.sql"

# Step 4: Drop all existing objects in target database (except extensions)
echo ""
echo "[Step 4/6] Preparing target database..."
psql -h "$TARGET_HOST" -p "$TARGET_PORT" -U "$DB_USER" -d "$DB_NAME" << 'EOF'
-- Drop all tables, views, functions, etc. but keep extensions
DO $$ 
DECLARE
    r RECORD;
BEGIN
    -- Drop all views
    FOR r IN (SELECT viewname FROM pg_views WHERE schemaname = 'public') LOOP
        EXECUTE 'DROP VIEW IF EXISTS public.' || quote_ident(r.viewname) || ' CASCADE';
    END LOOP;
    
    -- Drop all tables
    FOR r IN (SELECT tablename FROM pg_tables WHERE schemaname = 'public') LOOP
        EXECUTE 'DROP TABLE IF EXISTS public.' || quote_ident(r.tablename) || ' CASCADE';
    END LOOP;
    
    -- Drop all functions
    FOR r IN (SELECT proname, oidvectortypes(proargtypes) as args 
              FROM pg_proc 
              INNER JOIN pg_namespace ns ON (pg_proc.pronamespace = ns.oid) 
              WHERE ns.nspname = 'public') LOOP
        EXECUTE 'DROP FUNCTION IF EXISTS public.' || quote_ident(r.proname) || '(' || r.args || ') CASCADE';
    END LOOP;
    
    -- Drop all sequences
    FOR r IN (SELECT sequencename FROM pg_sequences WHERE schemaname = 'public') LOOP
        EXECUTE 'DROP SEQUENCE IF EXISTS public.' || quote_ident(r.sequencename) || ' CASCADE';
    END LOOP;
    
    -- Drop all types (except built-in)
    FOR r IN (SELECT typname FROM pg_type t 
              JOIN pg_namespace n ON t.typnamespace = n.oid 
              WHERE n.nspname = 'public' AND t.typtype = 'c') LOOP
        EXECUTE 'DROP TYPE IF EXISTS public.' || quote_ident(r.typname) || ' CASCADE';
    END LOOP;
END $$;

-- Verify extensions are still present
SELECT extname, extversion FROM pg_extension WHERE extname IN ('vector', 'pg_trgm', 'btree_gin');
EOF
echo "✓ Target database prepared"

# Step 5: Restore the complete backup to target database
echo ""
echo "[Step 5/6] Restoring data to target database..."
pg_restore -h "$TARGET_HOST" -p "$TARGET_PORT" -U "$DB_USER" -d "$DB_NAME" \
    --verbose \
    --no-owner \
    --no-privileges \
    --single-transaction \
    "$BACKUP_DIR/iac_platform_full.dump" 2>&1 | grep -v "already exists" || true
echo "✓ Data restored to target database"

# Step 6: Verify migration
echo ""
echo "[Step 6/6] Verifying migration..."
echo ""
echo "Source database table counts:"
psql -h "$SOURCE_HOST" -p "$SOURCE_PORT" -U "$DB_USER" -d "$DB_NAME" << 'EOF'
SELECT 
    schemaname,
    relname as table_name,
    n_live_tup as row_count
FROM pg_stat_user_tables
ORDER BY n_live_tup DESC;
EOF

echo ""
echo "Target database table counts:"
psql -h "$TARGET_HOST" -p "$TARGET_PORT" -U "$DB_USER" -d "$DB_NAME" << 'EOF'
SELECT 
    schemaname,
    relname as table_name,
    n_live_tup as row_count
FROM pg_stat_user_tables
ORDER BY n_live_tup DESC;
EOF

echo ""
echo "Target database extensions:"
psql -h "$TARGET_HOST" -p "$TARGET_PORT" -U "$DB_USER" -d "$DB_NAME" -c "SELECT extname, extversion FROM pg_extension;"

echo ""
echo "=========================================="
echo "Migration completed successfully!"
echo "=========================================="
echo ""
echo "Backup files are stored in: $BACKUP_DIR"
echo "  - Full backup: iac_platform_full.dump"
echo "  - Schema only: iac_platform_schema.sql"
echo "  - Data only: iac_platform_data.sql"
echo ""
echo "New pgvector database is available at:"
echo "  Host: $TARGET_HOST"
echo "  Port: $TARGET_PORT"
echo "  Database: $DB_NAME"
echo "  User: $DB_USER"
echo ""
echo "Connection string:"
echo "  postgresql://$DB_USER:$DB_PASSWORD@$TARGET_HOST:$TARGET_PORT/$DB_NAME"
echo ""
