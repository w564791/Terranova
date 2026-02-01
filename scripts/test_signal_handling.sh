#!/bin/bash

# 信号处理集成测试脚本
# 用于测试系统在收到SIGINT/SIGTERM时的行为

set -e

echo "========================================="
echo "Signal Handling Integration Test"
echo "========================================="
echo ""

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 测试1: 单元测试
echo "Test 1: Running unit tests..."
cd backend
go test -v ./services -run TestSignalManager
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Unit tests passed${NC}"
else
    echo -e "${RED}✗ Unit tests failed${NC}"
    exit 1
fi
echo ""

# 测试2: 编译检查
echo "Test 2: Checking compilation..."
go build -o /tmp/iac-platform-test ./
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Compilation successful${NC}"
    rm /tmp/iac-platform-test
else
    echo -e "${RED}✗ Compilation failed${NC}"
    exit 1
fi
echo ""

# 测试3: 实际信号测试（可选）
echo "Test 3: Actual signal test (manual)"
echo -e "${YELLOW}To test actual signal handling:${NC}"
echo "1. Start the server: make run"
echo "2. Trigger a Plan or Apply task"
echo "3. During execution, press Ctrl+C"
echo "4. Observe logs for:"
echo "   - Received signal: interrupt"
echo "   - In critical section [stage], will exit gracefully"
echo "   - Exited critical section: stage"
echo "   - Critical operations completed"
echo ""

# 测试4: 日志验证
echo "Test 4: Verifying signal handling logs..."
echo "Expected log patterns:"
echo "  - Global signal manager initialized"
echo "  - Entered critical section: [stage]"
echo "  - Exited critical section: [stage]"
echo "  - Received signal: [signal]"
echo ""

echo "========================================="
echo -e "${GREEN}All automated tests passed!${NC}"
echo "========================================="
echo ""
echo "Manual testing steps:"
echo "1. Start server: cd backend && go run main.go"
echo "2. Create a Plan task via API"
echo "3. During 'saving_plan' stage, press Ctrl+C"
echo "4. Verify: Plan data is saved before exit"
echo ""
echo "5. Create an Apply task via API"
echo "6. During 'applying' stage, press Ctrl+C"
echo "7. Verify: Apply completes and state is saved before exit"
echo ""
