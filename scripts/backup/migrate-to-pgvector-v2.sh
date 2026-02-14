#!/bin/bash

# PostgreSQL to pgvector Migration Script (Version 2)
# Uses container-internal pg_dump/pg_restore to ensure version compatibility
# This script migrates all data from the original PostgreSQL database to the new pgvector database

set -e

# Configuration
SOURCE_CONTAINER="iac-platform-postgres"
TARGET_CONTAINER="iac-platform-postgres-pgvector"
DB_NAME="iac_platform"
DB_USER="postgres"
DB_PASSWORD="postgres123"

# Backup directory inside containers
BACKUP_DIR="/tmp/migration"

echo "=========================================="
echo "PostgreSQL to pgvector Migration Script v2"
echo "=========================================="
echo ""
echo "Source Container: $SOURCE_CONTAINER"
echo "Target Container: $TARGET_CONTAINER"
echo "Database: $DB_NAME"
echo ""

# Step 1: Create backup directory in source container
echo "[Step 1/7] Preparing source container..."
docker exec $SOURCE_CONTAINER mkdir -p $BACKUP_DIR
echo "✓ Backup directory created"

# Step 2: Create a complete backup from source database using container's pg_dump
echo ""
echo "[Step 2/7] Creating complete backup from source database..."
docker exec -e PGPASSWORD=$DB_PASSWORD $SOURCE_CONTAINER pg_dump \
    -h localhost -U $DB_USER -d $DB_NAME \
    --format=custom \
    --verbose \
    --no-owner \
    --no-privileges \
    -f $BACKUP_DIR/iac_platform_full.dump 2>&1
echo "✓ Full backup created"

# Step 3: Export schema only (for reference)
echo ""
echo "[Step 3/7] Exporting schema (tables, indexes, functions, triggers)..."
docker exec -e PGPASSWORD=$DB_PASSWORD $SOURCE_CONTAINER pg_dump \
    -h localhost -U $DB_USER -d $DB_NAME \
    --schema-only \
    --no-owner \
    --no-privileges \
    -f $BACKUP_DIR/iac_platform_schema.sql
echo "✓ Schema exported"

# Step 4: Copy backup files from source to target container
echo ""
echo "[Step 4/7] Copying backup files to target container..."
docker exec $TARGET_CONTAINER mkdir -p $BACKUP_DIR
docker cp $SOURCE_CONTAINER:$BACKUP_DIR/iac_platform_full.dump /tmp/iac_platform_full.dump
docker cp /tmp/iac_platform_full.dump $TARGET_CONTAINER:$BACKUP_DIR/iac_platform_full.dump
docker cp $SOURCE_CONTAINER:$BACKUP_DIR/iac_platform_schema.sql /tmp/iac_platform_schema.sql
docker cp /tmp/iac_platform_schema.sql $TARGET_CONTAINER:$BACKUP_DIR/iac_platform_schema.sql
echo "✓ Backup files copied"

# Step 5: Drop all existing objects in target database (except extensions)
echo ""
echo "[Step 5/7] Preparing target database..."
docker exec -e PGPASSWORD=$DB_PASSWORD $TARGET_CONTAINER psql -h localhost -U $DB_USER -d $DB_NAME << 'EOF'
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
    
    -- Drop all sequences
    FOR r IN (SELECT sequencename FROM pg_sequences WHERE schemaname = 'public') LOOP
        EXECUTE 'DROP SEQUENCE IF EXISTS public.' || quote_ident(r.sequencename) || ' CASCADE';
    END LOOP;
END $$;

-- Verify extensions are still present
SELECT extname, extversion FROM pg_extension WHERE extname IN ('vector', 'pg_trgm', 'btree_gin');
EOF
echo "✓ Target database prepared"

# Step 6: Restore the complete backup to target database
echo ""
echo "[Step 6/7] Restoring data to target database..."
docker exec -e PGPASSWORD=$DB_PASSWORD $TARGET_CONTAINER pg_restore \
    -h localhost -U $DB_USER -d $DB_NAME \
    --verbose \
    --no-owner \
    --no-privileges \
    --single-transaction \
    $BACKUP_DIR/iac_platform_full.dump 2>&1 || true
echo "✓ Data restored to target database"

# Step 7: Verify migration
echo ""
echo "[Step 7/7] Verifying migration..."
echo ""
echo "Source database table counts:"
docker exec -e PGPASSWORD=$DB_PASSWORD $SOURCE_CONTAINER psql -h localhost -U $DB_USER -d $DB_NAME << 'EOF'
SELECT 
    schemaname,
    relname as table_name,
    n_live_tup as row_count
FROM pg_stat_user_tables
WHERE n_live_tup > 0
ORDER BY n_live_tup DESC
LIMIT 20;
EOF

echo ""
echo "Target database table counts:"
docker exec -e PGPASSWORD=$DB_PASSWORD $TARGET_CONTAINER psql -h localhost -U $DB_USER -d $DB_NAME << 'EOF'
SELECT 
    schemaname,
    relname as table_name,
    n_live_tup as row_count
FROM pg_stat_user_tables
WHERE n_live_tup > 0
ORDER BY n_live_tup DESC
LIMIT 20;
EOF

echo ""
echo "Target database extensions:"
docker exec -e PGPASSWORD=$DB_PASSWORD $TARGET_CONTAINER psql -h localhost -U $DB_USER -d $DB_NAME -c "SELECT extname, extversion FROM pg_extension;"

echo ""
echo "Target database total tables:"
docker exec -e PGPASSWORD=$DB_PASSWORD $TARGET_CONTAINER psql -h localhost -U $DB_USER -d $DB_NAME -c "SELECT COUNT(*) as table_count FROM pg_tables WHERE schemaname = 'public';"

echo ""
echo "=========================================="
echo "Migration completed!"
echo "=========================================="
echo ""
echo "New pgvector database is available at:"
echo "  Host: localhost"
echo "  Port: 15432"
echo "  Database: $DB_NAME"
echo "  User: $DB_USER"
echo ""
echo "Connection string:"
echo "  postgresql://$DB_USER:$DB_PASSWORD@localhost:15432/$DB_NAME"
echo ""

# Cleanup
echo "Cleaning up temporary files..."
rm -f /tmp/iac_platform_full.dump /tmp/iac_platform_schema.sql
echo "✓ Cleanup completed"
