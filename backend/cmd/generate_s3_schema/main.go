package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"iac-platform/demo"
)

func main() {
	// 直接调用demo包的GetS3ModuleSchema函数获取完整的S3 schema
	s3Schema := demo.GetS3ModuleSchema()

	// 将Schema转换为JSON
	schemaJSON, err := json.MarshalIndent(s3Schema, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal schema: %v", err)
	}

	// 打印JSON（可以直接复制到数据库）
	fmt.Println("=== S3 Module Schema JSON ===")
	fmt.Println(string(schemaJSON))

	// 保存到文件
	err = os.WriteFile("s3_schema.json", schemaJSON, 0644)
	if err != nil {
		log.Fatalf("Failed to write file: %v", err)
	}

	fmt.Println("\n=== Schema saved to s3_schema.json ===")
	fmt.Printf("Total fields: %d\n", countFields(s3Schema.Schema))

	// 生成SQL插入语句
	generateInsertSQL(s3Schema)
}

// countFields 统计Schema中的字段数量
func countFields(schema interface{}) int {
	// 使用反射统计S3Module结构体的字段数
	schemaJSON, _ := json.Marshal(schema)
	var schemaMap map[string]interface{}
	json.Unmarshal(schemaJSON, &schemaMap)
	return len(schemaMap)
}

// generateInsertSQL 生成用于插入数据库的SQL语句
func generateInsertSQL(s3Schema demo.S3ModuleSchema) {
	// 只取Schema部分作为schema_data
	schemaData, _ := json.Marshal(s3Schema.Schema)

	fmt.Println("\n=== SQL Insert Statement ===")
	fmt.Printf(`
INSERT INTO schemas (module_id, schema_data, version, status, ai_generated, created_by)
VALUES (
    6,  -- S3 module ID
    '%s'::jsonb,
    '2.0.0',
    'active',
    false,  -- 这是demo数据，不是AI生成的
    1  -- admin user
);
`, string(schemaData))
}
