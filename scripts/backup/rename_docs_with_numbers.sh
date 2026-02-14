#!/bin/bash

# 为 docs 目录下的文件添加序号前缀
# README.md 和子目录不添加序号

cd "$(dirname "$0")/../docs" || exit 1

echo "开始为文档添加序号..."

# 快速开始与核心指南 (01-09)
[ -f "QUICK_START_FOR_AI.md" ] && mv "QUICK_START_FOR_AI.md" "01-QUICK_START_FOR_AI.md"
[ -f "EXECUTION_GUIDE.md" ] && mv "EXECUTION_GUIDE.md" "02-EXECUTION_GUIDE.md"
[ -f "development-guide.md" ] && mv "development-guide.md" "03-development-guide.md"
[ -f "testing-guide.md" ] && mv "testing-guide.md" "04-testing-guide.md"
[ -f "project-status.md" ] && mv "project-status.md" "05-project-status.md"
[ -f "project-completion-summary.md" ] && mv "project-completion-summary.md" "06-project-completion-summary.md"
[ -f "FINAL_IMPLEMENTATION_STATUS.md" ] && mv "FINAL_IMPLEMENTATION_STATUS.md" "07-FINAL_IMPLEMENTATION_STATUS.md"

# 架构与设计 (10-19)
[ -f "agent-architecture.md" ] && mv "agent-architecture.md" "10-agent-architecture.md"
[ -f "id-specification.md" ] && mv "id-specification.md" "11-id-specification.md"
[ -f "database-schema.sql" ] && mv "database-schema.sql" "12-database-schema.sql"
[ -f "database-schema-fixed.sql" ] && mv "database-schema-fixed.sql" "13-database-schema-fixed.sql"

# 安全与权限 (20-39)
[ -f "CRITICAL-SECURITY-VULNERABILITY-REPORT.md" ] && mv "CRITICAL-SECURITY-VULNERABILITY-REPORT.md" "20-CRITICAL-SECURITY-VULNERABILITY-REPORT.md"
[ -f "authentication-injection-security-audit.md" ] && mv "authentication-injection-security-audit.md" "21-authentication-injection-security-audit.md"
[ -f "router-authentication-audit-report.md" ] && mv "router-authentication-audit-report.md" "22-router-authentication-audit-report.md"
[ -f "permission-fix-complete-report.md" ] && mv "permission-fix-complete-report.md" "23-permission-fix-complete-report.md"
[ -f "permission-fix-final-report.md" ] && mv "permission-fix-final-report.md" "24-permission-fix-final-report.md"
[ -f "permission-fix-implementation-plan.md" ] && mv "permission-fix-implementation-plan.md" "25-permission-fix-implementation-plan.md"
[ -f "permission-fix-progress-update.md" ] && mv "permission-fix-progress-update.md" "26-permission-fix-progress-update.md"
[ -f "permission-fix-summary-report.md" ] && mv "permission-fix-summary-report.md" "27-permission-fix-summary-report.md"
[ -f "duplicate-permission-fix.md" ] && mv "duplicate-permission-fix.md" "28-duplicate-permission-fix.md"
[ -f "semantic-id-implementation-guide.md" ] && mv "semantic-id-implementation-guide.md" "29-semantic-id-implementation-guide.md"
[ -f "semantic-id-implementation-complete.md" ] && mv "semantic-id-implementation-complete.md" "30-semantic-id-implementation-complete.md"
[ -f "router-permission-ids-checklist.md" ] && mv "router-permission-ids-checklist.md" "31-router-permission-ids-checklist.md"

# 后端开发 (40-59)
[ -f "backend-development-guide.md" ] && mv "backend-development-guide.md" "40-backend-development-guide.md"
[ -f "golang-development-guide.md" ] && mv "golang-development-guide.md" "41-golang-development-guide.md"
[ -f "backend-route-optimization-plan.md" ] && mv "backend-route-optimization-plan.md" "42-backend-route-optimization-plan.md"
[ -f "route-restructure-plan.md" ] && mv "route-restructure-plan.md" "43-route-restructure-plan.md"
[ -f "api-specification.md" ] && mv "api-specification.md" "44-api-specification.md"
[ -f "api-test-results.md" ] && mv "api-test-results.md" "45-api-test-results.md"
[ -f "swagger-implementation-guide.md" ] && mv "swagger-implementation-guide.md" "46-swagger-implementation-guide.md"
[ -f "swagger-completion-guide.md" ] && mv "swagger-completion-guide.md" "47-swagger-completion-guide.md"
[ -f "swagger-progress-summary.md" ] && mv "swagger-progress-summary.md" "48-swagger-progress-summary.md"
[ -f "swagger-apis-checklist.md" ] && mv "swagger-apis-checklist.md" "49-swagger-apis-checklist.md"

# 前端开发 (60-69)
[ -f "frontend-debugging-guide.md" ] && mv "frontend-debugging-guide.md" "60-frontend-debugging-guide.md"
[ -f "frontend-form-style-guide.md" ] && mv "frontend-form-style-guide.md" "61-frontend-form-style-guide.md"
[ -f "new-page-template.md" ] && mv "new-page-template.md" "62-new-page-template.md"
[ -f "mobile-access-guide.md" ] && mv "mobile-access-guide.md" "63-mobile-access-guide.md"
[ -f "frontend-api-authorization-diagnosis.md" ] && mv "frontend-api-authorization-diagnosis.md" "64-frontend-api-authorization-diagnosis.md"
[ -f "resource-edit-refresh-fix.md" ] && mv "resource-edit-refresh-fix.md" "65-resource-edit-refresh-fix.md"
[ -f "personal-settings-implementation-guide.md" ] && mv "personal-settings-implementation-guide.md" "66-personal-settings-implementation-guide.md"

# 工作空间 (70-79)
[ -f "workspace-detail-integration-guide.md" ] && mv "workspace-detail-integration-guide.md" "70-workspace-detail-integration-guide.md"
[ -f "workspace-enhancement-complete-guide.md" ] && mv "workspace-enhancement-complete-guide.md" "71-workspace-enhancement-complete-guide.md"
[ -f "workspace-module-status.md" ] && mv "workspace-module-status.md" "72-workspace-module-status.md"

# 模块管理 (80-89)
[ -f "module-import-optimization.md" ] && mv "module-import-optimization.md" "80-module-import-optimization.md"
[ -f "module-import-realtime-check-guide.md" ] && mv "module-import-realtime-check-guide.md" "81-module-import-realtime-check-guide.md"
[ -f "demo-implementation-summary.md" ] && mv "demo-implementation-summary.md" "82-demo-implementation-summary.md"
[ -f "demo-module-development-guide.md" ] && mv "demo-module-development-guide.md" "83-demo-module-development-guide.md"
[ -f "s3-demo-verification-guide.md" ] && mv "s3-demo-verification-guide.md" "84-s3-demo-verification-guide.md"
[ -f "s3-module-demo-guide.md" ] && mv "s3-module-demo-guide.md" "85-s3-module-demo-guide.md"

# Schema 管理 (90-99)
[ -f "dynamic-schema-testing-guide.md" ] && mv "dynamic-schema-testing-guide.md" "90-dynamic-schema-testing-guide.md"
[ -f "nested-schema-rendering-guide.md" ] && mv "nested-schema-rendering-guide.md" "91-nested-schema-rendering-guide.md"
[ -f "schema-edit-feature-guide.md" ] && mv "schema-edit-feature-guide.md" "92-schema-edit-feature-guide.md"
[ -f "schema-import-capability-4-guide.md" ] && mv "schema-import-capability-4-guide.md" "93-schema-import-capability-4-guide.md"
[ -f "s3-complete-schema.json" ] && mv "s3-complete-schema.json" "94-s3-complete-schema.json"
[ -f "tf-file-parser-guide.md" ] && mv "tf-file-parser-guide.md" "95-tf-file-parser-guide.md"

# AI 功能 (100-109)
[ -f "ai-development-guide.md" ] && mv "ai-development-guide.md" "100-ai-development-guide.md"

# 功能特性 (110-119)
[ -f "feature-toggle-guide.md" ] && mv "feature-toggle-guide.md" "110-feature-toggle-guide.md"
[ -f "hidden-default-feature-guide.md" ] && mv "hidden-default-feature-guide.md" "111-hidden-default-feature-guide.md"
[ -f "vcs-integration.md" ] && mv "vcs-integration.md" "112-vcs-integration.md"
[ -f "user-team-id-implementation-summary.md" ] && mv "user-team-id-implementation-summary.md" "113-user-team-id-implementation-summary.md"
[ -f "user-team-id-migration-status.md" ] && mv "user-team-id-migration-status.md" "114-user-team-id-migration-status.md"
[ -f "user-team-id-optimization-plan.md" ] && mv "user-team-id-optimization-plan.md" "115-user-team-id-optimization-plan.md"
[ -f "team-token-implementation-summary.md" ] && mv "team-token-implementation-summary.md" "116-team-token-implementation-summary.md"

echo "文档重命名完成！"
echo "注意: README.md 和子目录未添加序号"
