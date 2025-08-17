#!/bin/bash

# WebP Image Proxy Service - 自动化测试脚本
# 用于 CI/CD 环境的测试运行器

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 测试结果统计
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
TEST_RESULTS=""

# 启动服务
echo "Starting WebP Image Proxy Service..."
./webpimg &
SERVER_PID=$!
echo "Server PID: $SERVER_PID"

# 等待服务启动
sleep 5

# 检查服务是否启动成功
if ! curl -s http://localhost:8080 > /dev/null; then
    echo -e "${RED}Failed to start server${NC}"
    exit 1
fi

echo -e "${GREEN}Server started successfully${NC}"

# 函数：运行单个测试
run_test() {
    local test_name=$1
    local test_command=$2
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    echo -n "Running test: $test_name... "
    
    if eval "$test_command" > /dev/null 2>&1; then
        PASSED_TESTS=$((PASSED_TESTS + 1))
        echo -e "${GREEN}✓ PASSED${NC}"
        TEST_RESULTS="${TEST_RESULTS}✅ ${test_name}\n"
        return 0
    else
        FAILED_TESTS=$((FAILED_TESTS + 1))
        echo -e "${RED}✗ FAILED${NC}"
        TEST_RESULTS="${TEST_RESULTS}❌ ${test_name}\n"
        return 1
    fi
}

# 测试1: 健康检查
run_test "Health Check" "curl -f -s http://localhost:8080/stats"

# 测试2: 缓存页面访问
run_test "Cache Page Access" "curl -f -s http://localhost:8080/cache"

# 测试3: 图片代理功能
TEST_IMAGE="https://via.placeholder.com/100"
run_test "Image Proxy" "curl -f -s -o /dev/null http://localhost:8080/${TEST_IMAGE}"

# 测试4: WebP 转换
run_test "WebP Conversion" "curl -f -s -H 'Accept: image/webp' http://localhost:8080/${TEST_IMAGE} | file - | grep -q WebP"

# 测试5: 缓存统计API
run_test "Stats API JSON" "curl -f -s http://localhost:8080/stats | jq -e '.cache_stats'"

# 测试6: 缓存命中
run_test "Cache Hit Test" "curl -f -s -o /dev/null http://localhost:8080/${TEST_IMAGE} && curl -f -s -o /dev/null http://localhost:8080/${TEST_IMAGE}"

# 测试7: 带参数的图片请求
run_test "Image with Width Parameter" "curl -f -s -o /dev/null 'http://localhost:8080/${TEST_IMAGE}?w=50'"

# 测试8: 带质量参数的请求
run_test "Image with Quality Parameter" "curl -f -s -o /dev/null 'http://localhost:8080/${TEST_IMAGE}?q=80'"

# 测试9: 缩略图生成
run_test "Thumbnail Generation" "curl -f -s -o /dev/null 'http://localhost:8080/${TEST_IMAGE}?w=32&h=32'"

# 测试10: 缓存控制API（需要密码）
if [ -f ".pass" ]; then
    ADMIN_PASS=$(cat .pass)
    PASS_HASH=$(echo -n "$ADMIN_PASS" | md5sum | cut -d' ' -f1)
    run_test "Cache Control API" "curl -f -s -X POST 'http://localhost:8080/cache/control' -H 'X-Admin-Password: $PASS_HASH' -d 'action=stats'"
fi

# 清理：停止服务
echo "Stopping server..."
kill $SERVER_PID 2>/dev/null || true
wait $SERVER_PID 2>/dev/null || true

# 生成测试报告
echo ""
echo "======================================"
echo "         TEST SUMMARY REPORT          "
echo "======================================"
echo -e "Total Tests: ${TOTAL_TESTS}"
echo -e "Passed: ${GREEN}${PASSED_TESTS}${NC}"
echo -e "Failed: ${RED}${FAILED_TESTS}${NC}"
echo ""
echo "Test Results:"
echo -e "$TEST_RESULTS"

# 生成 Markdown 报告
cat > test-report.md << EOF
# Test Report

## Summary
- **Total Tests**: ${TOTAL_TESTS}
- **Passed**: ${PASSED_TESTS}
- **Failed**: ${FAILED_TESTS}
- **Success Rate**: $(( PASSED_TESTS * 100 / TOTAL_TESTS ))%

## Test Results
${TEST_RESULTS}

## Environment
- **Date**: $(date -u +"%Y-%m-%d %H:%M:%S UTC")
- **Go Version**: $(go version)
- **OS**: $(uname -s)
- **Architecture**: $(uname -m)
EOF

# 退出码
if [ $FAILED_TESTS -gt 0 ]; then
    echo -e "${RED}Tests failed!${NC}"
    exit 1
else
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
fi