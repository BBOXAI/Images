#!/bin/bash

PORT=8085
BASE_URL="http://localhost:$PORT"
TEST_IMAGE="/storage/c09cc20fac2781ba31542cd1116939d40398d7ab.jpg"

echo "=========================================="
echo "图片变换与缓存层级测试"
echo "=========================================="

# 测试原始图片
echo -e "\n1. 获取原始图片："
curl -s -I "$BASE_URL$TEST_IMAGE" | grep -E "X-Cache|X-Storage|X-Transform|X-Image|Content-Length|Content-Type"

# 测试WebP转换
echo -e "\n2. 转换为WebP格式："
curl -s -I "$BASE_URL$TEST_IMAGE?format=webp" | grep -E "X-Cache|X-Storage|X-Transform|X-Image|Content-Length|Content-Type"

# 测试尺寸调整（缩小到200x200）
echo -e "\n3. 调整尺寸为200x200（fit模式）："
curl -s -I "$BASE_URL$TEST_IMAGE?w=200&h=200" | grep -E "X-Cache|X-Storage|X-Transform|X-Image|Content-Length|Content-Type"

# 再次获取相同的变换（应该从缓存）
echo -e "\n4. 再次获取200x200（应该从缓存）："
curl -s -I "$BASE_URL$TEST_IMAGE?w=200&h=200" | grep -E "X-Cache|X-Storage|X-Transform|X-Image|Content-Length|Content-Type"

# 测试不同的缩放模式
echo -e "\n5. 200x200 fill模式（裁剪）："
curl -s -I "$BASE_URL$TEST_IMAGE?w=200&h=200&mode=fill" | grep -E "X-Cache|X-Storage|X-Transform|X-Image|Content-Length|Content-Type"

# 测试质量调整
echo -e "\n6. JPEG格式，质量50%："
curl -s -I "$BASE_URL$TEST_IMAGE?format=jpeg&q=50" | grep -E "X-Cache|X-Storage|X-Transform|X-Image|Content-Length|Content-Type"

# 组合变换：缩放 + WebP
echo -e "\n7. 缩放到300x300并转换为WebP："
curl -s -I "$BASE_URL$TEST_IMAGE?w=300&h=300&format=webp" | grep -E "X-Cache|X-Storage|X-Transform|X-Image|Content-Length|Content-Type"

# 再次获取组合变换（测试缓存）
echo -e "\n8. 再次获取300x300 WebP（应该从缓存）："
curl -s -I "$BASE_URL$TEST_IMAGE?w=300&h=300&format=webp" | grep -E "X-Cache|X-Storage|X-Transform|X-Image|Content-Length|Content-Type"

echo -e "\n=========================================="
echo "缓存状态说明："
echo "- HIT-MEMORY: 从内存缓存获取"
echo "- HIT-MEMORY-TRANSFORM: 从内存获取变换后的缓存"
echo "- HIT-LOCAL: 从本地磁盘获取"
echo "- HIT-LOCAL-TRANSFORM: 从本地获取变换后的缓存"
echo "- TRANSFORM-ON-DEMAND: 实时变换（首次请求）"
echo "=========================================="