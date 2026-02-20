#!/bin/bash

# 为 docs 子目录下的文件添加序号前缀
# README.md 不添加序号

cd "$(dirname "$0")/../docs" || exit 1

echo "开始为子目录文档添加序号..."

# ========== admin 目录 ==========
cd admin 2>/dev/null && {
    echo "处理 admin 目录..."
    [ -f "development-progress.md" ] && mv "development-progress.md" "04-development-progress.md"
    cd ..
}

# ========== ai 目录 ==========
cd ai 2>/dev/null && {
    echo "处理 ai 目录..."
    [ -f "ai-error-analysis-design.md" ] && mv "ai-error-analysis-design.md" "01-ai-error-analysis-design.md"
    [ -f "ai-error-analysis-implementation-progress.md" ] && mv "ai-error-analysis-implementation-progress.md" "02-ai-error-analysis-implementation-progress.md"
    [ -f "ai-error-analysis-user-guide.md" ] && mv "ai-error-analysis-user-guide.md" "03-ai-error-analysis-user-guide.md"
    [ -f "ai-provider-capability-management.md" ] && mv "ai-provider-capability-management.md" "04-ai-provider-capability-management.md"
    [ -f "openai-compatible-support.md" ] && mv "openai-compatible-support.md" "05-openai-compatible-support.md"
    cd ..
}

# ========== iam 目录 ==========
cd iam 2>/dev/null && {
    echo "处理 iam 目录..."
    [ -f "permission-system-design-FirstDraft.md" ] && mv "permission-system-design-FirstDraft.md" "01-permission-system-design-FirstDraft.md"
    [ -f "iac-platform-permission-system-design.md" ] && mv "iac-platform-permission-system-design.md" "02-iac-platform-permission-system-design.md"
    [ -f "iac-platform-permission-system-design-v2.md" ] && mv "iac-platform-permission-system-design-v2.md" "03-iac-platform-permission-system-design-v2.md"
    [ -f "iam-roles-guide.md" ] && mv "iam-roles-guide.md" "04-iam-roles-guide.md"
    [ -f "role-implementation-plan.md" ] && mv "role-implementation-plan.md" "05-role-implementation-plan.md"
    [ -f "fine-grained-permissions-priority.md" ] && mv "fine-grained-permissions-priority.md" "06-fine-grained-permissions-priority.md"
    [ -f "permission-granularity-priority.md" ] && mv "permission-granularity-priority.md" "07-permission-granularity-priority.md"
    [ -f "workspace-permissions-analysis.md" ] && mv "workspace-permissions-analysis.md" "08-workspace-permissions-analysis.md"
    [ -f "optimized-workspace-permissions.md" ] && mv "optimized-workspace-permissions.md" "09-optimized-workspace-permissions.md"
    [ -f "workspace-management-consolidation.md" ] && mv "workspace-management-consolidation.md" "10-workspace-management-consolidation.md"
    [ -f "workspace-management-final.md" ] && mv "workspace-management-final.md" "11-workspace-management-final.md"
    [ -f "workspace-task-permissions.md" ] && mv "workspace-task-permissions.md" "12-workspace-task-permissions.md"
    [ -f "workspace-permissions-implementation-complete.md" ] && mv "workspace-permissions-implementation-complete.md" "13-workspace-permissions-implementation-complete.md"
    [ -f "admin-ui-prototype.md" ] && mv "admin-ui-prototype.md" "14-admin-ui-prototype.md"
    [ -f "implementation-progress.md" ] && mv "implementation-progress.md" "15-implementation-progress.md"
    [ -f "DEVELOPMENT_STATUS.md" ] && mv "DEVELOPMENT_STATUS.md" "16-DEVELOPMENT_STATUS.md"
    [ -f "complete-implementation-summary.md" ] && mv "complete-implementation-summary.md" "17-complete-implementation-summary.md"
    [ -f "BACKEND_COMPLETION_SUMMARY.md" ] && mv "BACKEND_COMPLETION_SUMMARY.md" "18-BACKEND_COMPLETION_SUMMARY.md"
    [ -f "FRONTEND_COMPLETION_SUMMARY.md" ] && mv "FRONTEND_COMPLETION_SUMMARY.md" "19-FRONTEND_COMPLETION_SUMMARY.md"
    [ -f "IMPLEMENTATION_COMPLETE_SUMMARY.md" ] && mv "IMPLEMENTATION_COMPLETE_SUMMARY.md" "20-IMPLEMENTATION_COMPLETE_SUMMARY.md"
    [ -f "FINAL_IMPLEMENTATION_SUMMARY.md" ] && mv "FINAL_IMPLEMENTATION_SUMMARY.md" "21-FINAL_IMPLEMENTATION_SUMMARY.md"
    [ -f "IAM_API_DEPLOYMENT_GUIDE.md" ] && mv "IAM_API_DEPLOYMENT_GUIDE.md" "22-IAM_API_DEPLOYMENT_GUIDE.md"
    [ -f "IAM_API_OPTIMIZATION_REQUIREMENTS.md" ] && mv "IAM_API_OPTIMIZATION_REQUIREMENTS.md" "23-IAM_API_OPTIMIZATION_REQUIREMENTS.md"
    [ -f "INTEGRATION_GUIDE.md" ] && mv "INTEGRATION_GUIDE.md" "24-INTEGRATION_GUIDE.md"
    [ -f "TASKS.md" ] && mv "TASKS.md" "25-TASKS.md"
    cd ..
}

# ========== module 目录 ==========
cd module 2>/dev/null && {
    echo "处理 module 目录..."
    [ -f "FINAL_IMPLEMENTATION_STATUS.md" ] && mv "FINAL_IMPLEMENTATION_STATUS.md" "01-FINAL_IMPLEMENTATION_STATUS.md"
    [ -f "frontend-implementation-status.md" ] && mv "frontend-implementation-status.md" "02-frontend-implementation-status.md"
    [ -f "module-demo-implementation-summary.md" ] && mv "module-demo-implementation-summary.md" "03-module-demo-implementation-summary.md"
    [ -f "module-demo-management.md" ] && mv "module-demo-management.md" "04-module-demo-management.md"
    [ -f "demo-preview-implementation-plan.md" ] && mv "demo-preview-implementation-plan.md" "05-demo-preview-implementation-plan.md"
    [ -f "demo-selector-in-add-resources.md" ] && mv "demo-selector-in-add-resources.md" "06-demo-selector-in-add-resources.md"
    [ -f "demo-version-compare-implementation.md" ] && mv "demo-version-compare-implementation.md" "07-demo-version-compare-implementation.md"
    cd ..
}

# ========== schema 目录 ==========
cd schema 2>/dev/null && {
    echo "处理 schema 目录..."
    [ -f "json-editor-feature-design.md" ] && mv "json-editor-feature-design.md" "01-json-editor-feature-design.md"
    cd ..
}

# ========== workspace 目录 ==========
cd workspace 2>/dev/null && {
    echo "处理 workspace 目录..."
    [ -f "development-progress.md" ] && mv "development-progress.md" "51-development-progress.md"
    [ -f "terraform-execution-development-progress.md" ] && mv "terraform-execution-development-progress.md" "52-terraform-execution-development-progress.md"
    [ -f "terraform-log-streaming-implementation.md" ] && mv "terraform-log-streaming-implementation.md" "53-terraform-log-streaming-implementation.md"
    [ -f "log-streaming-complete-summary.md" ] && mv "log-streaming-complete-summary.md" "54-log-streaming-complete-summary.md"
    [ -f "log-streaming-final-summary.md" ] && mv "log-streaming-final-summary.md" "55-log-streaming-final-summary.md"
    [ -f "dynamicform-integration-complete.md" ] && mv "dynamicform-integration-complete.md" "56-dynamicform-integration-complete.md"
    [ -f "provider-settings-bug-fixes.md" ] && mv "provider-settings-bug-fixes.md" "57-provider-settings-bug-fixes.md"
    [ -f "resources-management-implementation.md" ] && mv "resources-management-implementation.md" "58-resources-management-implementation.md"
    [ -f "resource-clone-feature-implementation.md" ] && mv "resource-clone-feature-implementation.md" "59-resource-clone-feature-implementation.md"
    [ -f "resource-list-optimization-plan.md" ] && mv "resource-list-optimization-plan.md" "60-resource-list-optimization-plan.md"
    [ -f "resource-version-comparison-implementation.md" ] && mv "resource-version-comparison-implementation.md" "61-resource-version-comparison-implementation.md"
    [ -f "resource-version-comparison-implementation-v2.md" ] && mv "resource-version-comparison-implementation-v2.md" "62-resource-version-comparison-implementation-v2.md"
    [ -f "resource-version-final-implementation.md" ] && mv "resource-version-final-implementation.md" "63-resource-version-final-implementation.md"
    [ -f "resource-version-final-summary.md" ] && mv "resource-version-final-summary.md" "64-resource-version-final-summary.md"
    [ -f "runs-list-optimization-summary.md" ] && mv "runs-list-optimization-summary.md" "65-runs-list-optimization-summary.md"
    [ -f "runs-list-advanced-optimization.md" ] && mv "runs-list-advanced-optimization.md" "66-runs-list-advanced-optimization.md"
    [ -f "runs-list-final-optimization.md" ] && mv "runs-list-final-optimization.md" "67-runs-list-final-optimization.md"
    [ -f "runs-tab-fix-summary.md" ] && mv "runs-tab-fix-summary.md" "68-runs-tab-fix-summary.md"
    [ -f "taskdetail-ui-optimization-guide.md" ] && mv "taskdetail-ui-optimization-guide.md" "69-taskdetail-ui-optimization-guide.md"
    [ -f "taskdetail-future-enhancements.md" ] && mv "taskdetail-future-enhancements.md" "70-taskdetail-future-enhancements.md"
    [ -f "today-summary-2025-10-11.md" ] && mv "today-summary-2025-10-11.md" "71-today-summary-2025-10-11.md"
    
    # workspace/lock 子目录
    cd lock 2>/dev/null && {
        echo "处理 workspace/lock 目录..."
        # 这个目录的文件已经有序号了
        cd ..
    }
    cd ..
}

echo "子目录文档重命名完成！"
echo "注意: README.md 文件未添加序号"
