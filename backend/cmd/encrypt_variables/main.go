package main

import (
	"database/sql"
	"log"
	"os"

	"iac-platform/internal/crypto"

	_ "github.com/lib/pq"
)

func main() {
	// 从环境变量或命令行参数获取数据库连接字符串
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://postgres:postgres123@localhost:15433/iac_platform?sslmode=disable"
	}

	log.Printf("Connecting to database...")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// 测试连接
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Printf("✓ Database connection successful")

	// 1. 备份现有敏感变量
	log.Printf("Creating backup table...")
	_, err = db.Exec(`
		DROP TABLE IF EXISTS workspace_variables_backup;
		CREATE TABLE workspace_variables_backup AS 
		SELECT * FROM workspace_variables WHERE sensitive = true;
	`)
	if err != nil {
		log.Fatalf("Failed to create backup: %v", err)
	}

	var backupCount int
	db.QueryRow("SELECT COUNT(*) FROM workspace_variables_backup").Scan(&backupCount)
	log.Printf("✓ Backed up %d sensitive variables", backupCount)

	// 2. 查询所有需要加密的敏感变量（明文的）
	log.Printf("Querying sensitive variables to encrypt...")
	rows, err := db.Query(`
		SELECT id, key, value 
		FROM workspace_variables 
		WHERE sensitive = true AND value != '' AND value IS NOT NULL
	`)
	if err != nil {
		log.Fatalf("Failed to query variables: %v", err)
	}
	defer rows.Close()

	// 3. 加密并更新
	encryptedCount := 0
	skippedCount := 0
	errorCount := 0

	for rows.Next() {
		var id uint
		var key, value string
		if err := rows.Scan(&id, &key, &value); err != nil {
			log.Printf("Error scanning row: %v", err)
			errorCount++
			continue
		}

		// 检查是否已加密
		if crypto.IsEncrypted(value) {
			log.Printf("Variable %d (%s) is already encrypted, skipping", id, key)
			skippedCount++
			continue
		}

		// 加密
		encrypted, err := crypto.EncryptValue(value)
		if err != nil {
			log.Printf("Error encrypting variable %d (%s): %v", id, key, err)
			errorCount++
			continue
		}

		// 更新数据库
		_, err = db.Exec(`
			UPDATE workspace_variables 
			SET value = $1, updated_at = NOW()
			WHERE id = $2
		`, encrypted, id)
		if err != nil {
			log.Printf("Error updating variable %d (%s): %v", id, key, err)
			errorCount++
			continue
		}

		log.Printf("✓ Encrypted variable %d (%s)", id, key)
		encryptedCount++
	}

	if err := rows.Err(); err != nil {
		log.Fatalf("Error iterating rows: %v", err)
	}

	// 4. 输出统计
	log.Printf("\n========== Migration Summary ==========")
	log.Printf("Total backed up: %d", backupCount)
	log.Printf("Encrypted: %d", encryptedCount)
	log.Printf("Skipped (already encrypted): %d", skippedCount)
	log.Printf("Errors: %d", errorCount)
	log.Printf("=======================================\n")

	if errorCount > 0 {
		log.Printf("WARNING: %d errors occurred during migration", errorCount)
		log.Printf("Backup table 'workspace_variables_backup' is available for recovery")
		os.Exit(1)
	}

	log.Printf("✓ Migration completed successfully!")
	log.Printf("Backup table 'workspace_variables_backup' is available for verification")
	log.Printf("\nTo verify encryption, run:")
	log.Printf("  psql -d iac_platform -c \"SELECT id, key, value, sensitive FROM workspace_variables WHERE sensitive = true LIMIT 5;\"")
	log.Printf("\nTo rollback if needed, run:")
	log.Printf("  psql -d iac_platform -c \"UPDATE workspace_variables SET value = backup.value FROM workspace_variables_backup backup WHERE workspace_variables.id = backup.id;\"")
}
