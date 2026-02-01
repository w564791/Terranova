package main

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	// 从环境变量或命令行参数获取数据库连接字符串
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://postgres:postgres123@localhost:15432/iac_platform?sslmode=disable"
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

	// 1. 从备份恢复明文数据
	log.Printf("Restoring plaintext values from backup...")
	result, err := db.Exec(`
		UPDATE workspace_variables 
		SET value = backup.value, updated_at = NOW()
		FROM workspace_variables_backup backup
		WHERE workspace_variables.id = backup.id;
	`)
	if err != nil {
		log.Fatalf("Failed to restore from backup: %v", err)
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("✓ Restored %d variables from backup", rowsAffected)

	// 2. 删除备份表
	log.Printf("Cleaning up backup table...")
	_, err = db.Exec("DROP TABLE IF EXISTS workspace_variables_backup;")
	if err != nil {
		log.Printf("Warning: Failed to drop backup table: %v", err)
	} else {
		log.Printf("✓ Backup table dropped")
	}

	log.Printf("\n✓ Rollback completed successfully!")
	log.Printf("\nNow you can restart the service with the new encryption key.")
	log.Printf("The GORM hooks will automatically encrypt sensitive variables on next save.")
	log.Printf("\nTo manually trigger re-encryption, you can run:")
	log.Printf("  cd backend && go run cmd/encrypt_variables/main.go")
}
