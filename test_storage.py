#!/usr/bin/env python3
"""
WebP图片服务存储测试脚本
测试三层存储架构的各种场景
"""

import requests
import json
import time
import hashlib
import os
from io import BytesIO
from PIL import Image, ImageDraw, ImageFont
import random
import base64

# 服务配置
BASE_URL = "http://localhost:8081"
UPLOAD_URL = f"{BASE_URL}/api/upload"
STATS_URL = f"{BASE_URL}/stats"

def create_test_image(text="Test", size=(200, 200), color=None):
    """创建测试图片"""
    if color is None:
        color = (random.randint(0, 255), random.randint(0, 255), random.randint(0, 255))
    
    img = Image.new('RGB', size, color=color)
    draw = ImageDraw.Draw(img)
    
    # 添加文字
    text_color = (255, 255, 255) if sum(color) < 384 else (0, 0, 0)
    draw.text((10, 10), text, fill=text_color)
    
    # 添加一些随机线条使每张图片唯一
    for _ in range(5):
        x1, y1 = random.randint(0, size[0]), random.randint(0, size[1])
        x2, y2 = random.randint(0, size[0]), random.randint(0, size[1])
        draw.line([(x1, y1), (x2, y2)], fill=text_color, width=2)
    
    # 转换为字节
    buffer = BytesIO()
    img.save(buffer, format='PNG')
    return buffer.getvalue()

def upload_image(image_data, filename="test.png"):
    """上传图片到服务器"""
    files = {'images': (filename, image_data, 'image/png')}
    response = requests.post(UPLOAD_URL, files=files)
    return response

def get_image(url):
    """获取图片"""
    full_url = BASE_URL + url
    response = requests.get(full_url)
    return response

def get_stats():
    """获取统计信息"""
    response = requests.get(STATS_URL)
    return response.json() if response.status_code == 200 else None

def test_basic_upload():
    """测试基本上传功能"""
    print("\n=== 测试基本上传功能 ===")
    
    # 创建测试图片
    img_data = create_test_image("Upload Test")
    
    # 上传图片
    response = upload_image(img_data, "basic_test.png")
    print(f"上传响应状态: {response.status_code}")
    
    if response.status_code == 200:
        result = response.json()
        print(f"上传成功: {json.dumps(result, indent=2)}")
        
        # 尝试获取上传的图片
        if result.get('urls'):
            img_url = result['urls'][0]
            print(f"获取图片: {img_url}")
            img_response = get_image(img_url)
            print(f"获取状态: {img_response.status_code}")
            print(f"图片大小: {len(img_response.content)} bytes")
            print(f"Content-Type: {img_response.headers.get('Content-Type')}")
            
            # 测试WebP转换
            webp_url = img_url + "?format=webp"
            print(f"\n获取WebP格式: {webp_url}")
            webp_response = get_image(webp_url)
            print(f"WebP状态: {webp_response.status_code}")
            print(f"WebP大小: {len(webp_response.content)} bytes")
            print(f"Content-Type: {webp_response.headers.get('Content-Type')}")
        
        return True
    else:
        print(f"上传失败: {response.text}")
        return False

def test_duplicate_upload():
    """测试重复上传（去重功能）"""
    print("\n=== 测试重复上传（去重） ===")
    
    # 创建相同的图片
    img_data = create_test_image("Duplicate Test", color=(100, 100, 200))
    
    # 第一次上传
    response1 = upload_image(img_data, "duplicate1.png")
    if response1.status_code == 200:
        result1 = response1.json()
        url1 = result1['urls'][0] if result1.get('urls') else None
        print(f"第一次上传: {url1}")
    
    # 第二次上传相同图片
    response2 = upload_image(img_data, "duplicate2.png")
    if response2.status_code == 200:
        result2 = response2.json()
        url2 = result2['urls'][0] if result2.get('urls') else None
        print(f"第二次上传: {url2}")
        
        # 检查是否返回相同的存储ID
        if url1 and url2:
            # 提取ID部分
            id1 = url1.split('/')[-1].split('.')[0]
            id2 = url2.split('/')[-1].split('.')[0]
            print(f"ID1: {id1}")
            print(f"ID2: {id2}")
            if id1 == id2:
                print("✓ 去重成功：两次上传返回相同ID")
            else:
                print("✗ 去重失败：两次上传返回不同ID")

def test_batch_upload():
    """测试批量上传"""
    print("\n=== 测试批量上传 ===")
    
    # 创建多个图片
    files = []
    for i in range(3):
        img_data = create_test_image(f"Batch {i+1}")
        files.append(('images', (f'batch_{i+1}.png', img_data, 'image/png')))
    
    # 批量上传
    response = requests.post(UPLOAD_URL, files=files)
    print(f"批量上传状态: {response.status_code}")
    
    if response.status_code == 200:
        result = response.json()
        print(f"上传数量: {result.get('count', 0)}")
        print(f"返回URLs: {json.dumps(result.get('urls', []), indent=2)}")
        return True
    else:
        print(f"批量上传失败: {response.text}")
        return False

def test_cache_performance():
    """测试缓存性能"""
    print("\n=== 测试缓存性能 ===")
    
    # 上传一张测试图片
    img_data = create_test_image("Cache Test", size=(500, 500))
    response = upload_image(img_data, "cache_test.png")
    
    if response.status_code == 200:
        result = response.json()
        img_url = result['urls'][0] if result.get('urls') else None
        
        if img_url:
            # 第一次获取（冷缓存）
            start = time.time()
            response1 = get_image(img_url)
            time1 = time.time() - start
            print(f"第一次获取（可能需要从磁盘读取）: {time1*1000:.2f}ms")
            
            # 第二次获取（热缓存）
            start = time.time()
            response2 = get_image(img_url)
            time2 = time.time() - start
            print(f"第二次获取（从内存缓存）: {time2*1000:.2f}ms")
            
            # 第三次获取（确认缓存）
            start = time.time()
            response3 = get_image(img_url)
            time3 = time.time() - start
            print(f"第三次获取（从内存缓存）: {time3*1000:.2f}ms")
            
            if time2 < time1 * 0.5:  # 如果第二次比第一次快50%以上
                print("✓ 缓存加速效果明显")
            else:
                print("✗ 缓存加速效果不明显")

def test_storage_stats():
    """测试存储统计"""
    print("\n=== 存储统计信息 ===")
    
    stats = get_stats()
    if stats:
        print(f"总请求数: {stats.get('total_requests', 0)}")
        print(f"缓存命中: {stats.get('cache_hits', 0)}")
        print(f"缓存未命中: {stats.get('cache_misses', 0)}")
        
        hit_rate = 0
        if stats.get('cache_hits', 0) + stats.get('cache_misses', 0) > 0:
            hit_rate = stats.get('cache_hits', 0) / (stats.get('cache_hits', 0) + stats.get('cache_misses', 0)) * 100
        print(f"缓存命中率: {hit_rate:.2f}%")
        
        print(f"内存缓存条目: {stats.get('memory_cache_entries', 0)}")
        print(f"磁盘缓存大小: {stats.get('disk_cache_size_mb', 0):.2f} MB")
        print(f"磁盘缓存文件数: {stats.get('disk_cache_files', 0)}")
    else:
        print("无法获取统计信息")

def test_proxy_remote_image():
    """测试代理远程图片"""
    print("\n=== 测试代理远程图片 ===")
    
    # 使用一个公开的测试图片
    test_image_url = "https://via.placeholder.com/300x200.png"
    proxy_url = f"/?url={test_image_url}"
    
    print(f"代理URL: {proxy_url}")
    response = get_image(proxy_url)
    print(f"响应状态: {response.status_code}")
    
    if response.status_code == 200:
        print(f"图片大小: {len(response.content)} bytes")
        print(f"Content-Type: {response.headers.get('Content-Type')}")
        
        # 测试WebP转换
        webp_proxy_url = f"/?url={test_image_url}&format=webp"
        print(f"\n代理并转换为WebP: {webp_proxy_url}")
        webp_response = get_image(webp_proxy_url)
        print(f"WebP状态: {webp_response.status_code}")
        if webp_response.status_code == 200:
            print(f"WebP大小: {len(webp_response.content)} bytes")
            print(f"Content-Type: {webp_response.headers.get('Content-Type')}")
            
            # 计算压缩率
            compression = (1 - len(webp_response.content) / len(response.content)) * 100
            print(f"压缩率: {compression:.2f}%")

def test_image_formats():
    """测试不同图片格式"""
    print("\n=== 测试不同图片格式 ===")
    
    formats = [
        ('JPEG', 'jpeg'),
        ('PNG', 'png'),
        ('GIF', 'gif'),
    ]
    
    for format_name, ext in formats:
        print(f"\n测试 {format_name} 格式:")
        
        # 创建特定格式的图片
        img = Image.new('RGB', (200, 200), color=(100, 150, 200))
        buffer = BytesIO()
        img.save(buffer, format=format_name)
        img_data = buffer.getvalue()
        
        # 上传
        response = upload_image(img_data, f"test.{ext}")
        if response.status_code == 200:
            result = response.json()
            url = result['urls'][0] if result.get('urls') else None
            print(f"  上传成功: {url}")
            
            # 获取原格式
            orig_response = get_image(url)
            print(f"  原格式大小: {len(orig_response.content)} bytes")
            
            # 获取WebP格式
            webp_response = get_image(url + "?format=webp")
            if webp_response.status_code == 200:
                print(f"  WebP大小: {len(webp_response.content)} bytes")
                compression = (1 - len(webp_response.content) / len(orig_response.content)) * 100
                print(f"  压缩率: {compression:.2f}%")

def main():
    """主测试函数"""
    print("=" * 50)
    print("WebP图片服务 - 存储架构测试")
    print("=" * 50)
    
    # 确保服务正在运行
    try:
        response = requests.get(BASE_URL, timeout=2)
        print(f"✓ 服务正在运行: {BASE_URL}")
    except:
        print(f"✗ 无法连接到服务: {BASE_URL}")
        print("请确保服务已启动")
        return
    
    # 运行测试
    tests = [
        ("基本上传", test_basic_upload),
        ("重复上传", test_duplicate_upload),
        ("批量上传", test_batch_upload),
        ("缓存性能", test_cache_performance),
        ("图片格式", test_image_formats),
        ("远程代理", test_proxy_remote_image),
        ("统计信息", test_storage_stats),
    ]
    
    results = []
    for test_name, test_func in tests:
        try:
            print(f"\n{'='*50}")
            test_func()
            results.append((test_name, "✓ 通过"))
        except Exception as e:
            print(f"测试异常: {e}")
            results.append((test_name, f"✗ 失败: {e}"))
    
    # 打印测试总结
    print("\n" + "="*50)
    print("测试总结")
    print("="*50)
    for test_name, result in results:
        print(f"{test_name}: {result}")

if __name__ == "__main__":
    main()