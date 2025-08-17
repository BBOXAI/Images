package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	BASE_URL   = "http://localhost:8080"
	TEST_IMAGE = "https://obscura.ac.cn/wp-content/uploads/2024/07/qrcode_for_gh_d6cbcd5a67fc_258.jpg"
)

func getStats() (int, int) {
	resp, err := http.Get(BASE_URL + "/stats")
	if err != nil {
		return 0, 0
	}
	defer resp.Body.Close()
	
	var stats map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&stats)
	
	hits := 0
	misses := 0
	
	if cacheStats, ok := stats["cache_stats"].(map[string]interface{}); ok {
		if h, ok := cacheStats["hits"].(float64); ok {
			hits = int(h)
		}
		if m, ok := cacheStats["misses"].(float64); ok {
			misses = int(m)
		}
	}
	
	return hits, misses
}

func testRequest(testURL string, name string) {
	fmt.Printf("\n测试: %s\n", name)
	fmt.Printf("URL: %s\n", testURL)
	
	// 获取初始统计
	hitsBefore, missesBefore := getStats()
	
	// 第一次请求
	resp1, err := http.Get(testURL)
	if err != nil {
		fmt.Printf("❌ 请求失败: %v\n", err)
		return
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	
	time.Sleep(100 * time.Millisecond) // 等待缓存写入
	
	hitsAfter1, missesAfter1 := getStats()
	
	// 第二次请求（应该命中缓存）
	resp2, err := http.Get(testURL)
	if err != nil {
		fmt.Printf("❌ 第二次请求失败: %v\n", err)
		return
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	
	time.Sleep(100 * time.Millisecond)
	
	hitsAfter2, missesAfter2 := getStats()
	
	// 分析结果
	fmt.Printf("第一次请求: %d bytes\n", len(body1))
	fmt.Printf("  缓存变化: hits %d->%d, misses %d->%d\n", 
		hitsBefore, hitsAfter1, missesBefore, missesAfter1)
	
	if missesAfter1 > missesBefore {
		fmt.Printf("  ✅ 第一次请求触发了cache miss (预期)\n")
	} else {
		fmt.Printf("  ❌ 第一次请求没有触发cache miss\n")
	}
	
	fmt.Printf("第二次请求: %d bytes\n", len(body2))
	fmt.Printf("  缓存变化: hits %d->%d, misses %d->%d\n", 
		hitsAfter1, hitsAfter2, missesAfter1, missesAfter2)
	
	if hitsAfter2 > hitsAfter1 {
		fmt.Printf("  ✅ 第二次请求命中缓存 (预期)\n")
	} else {
		fmt.Printf("  ❌ 第二次请求没有命中缓存！\n")
	}
	
	// 检查内容是否一致
	if len(body1) == len(body2) {
		fmt.Printf("  ✅ 两次请求返回内容大小一致\n")
	} else {
		fmt.Printf("  ⚠️  两次请求返回内容大小不一致: %d vs %d\n", len(body1), len(body2))
	}
}

func main_test_cache() {
	fmt.Println("=== 缓存系统详细测试 ===")
	
	// 测试不同的请求组合
	tests := []struct {
		name string
		url  string
	}{
		{
			"基本请求（无参数）",
			fmt.Sprintf("%s/?url=%s", BASE_URL, url.QueryEscape(TEST_IMAGE)),
		},
		{
			"带宽度参数",
			fmt.Sprintf("%s/?url=%s&w=200", BASE_URL, url.QueryEscape(TEST_IMAGE)),
		},
		{
			"带高度参数",
			fmt.Sprintf("%s/?url=%s&h=200", BASE_URL, url.QueryEscape(TEST_IMAGE)),
		},
		{
			"带格式参数",
			fmt.Sprintf("%s/?url=%s&format=webp", BASE_URL, url.QueryEscape(TEST_IMAGE)),
		},
		{
			"组合参数",
			fmt.Sprintf("%s/?url=%s&w=150&h=150&format=webp", BASE_URL, url.QueryEscape(TEST_IMAGE)),
		},
	}
	
	for _, test := range tests {
		testRequest(test.url, test.name)
		time.Sleep(500 * time.Millisecond) // 避免请求过快
	}
	
	// 最终统计
	fmt.Println("\n=== 最终统计 ===")
	resp, _ := http.Get(BASE_URL + "/stats")
	defer resp.Body.Close()
	
	var stats map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&stats)
	
	if cacheStats, ok := stats["cache_stats"].(map[string]interface{}); ok {
		hits := 0
		misses := 0
		if h, ok := cacheStats["hits"].(float64); ok {
			hits = int(h)
		}
		if m, ok := cacheStats["misses"].(float64); ok {
			misses = int(m)
		}
		
		hitRate := float64(hits) / float64(hits+misses) * 100
		fmt.Printf("总缓存命中: %d\n", hits)
		fmt.Printf("总缓存未命中: %d\n", misses)
		fmt.Printf("命中率: %.1f%%\n", hitRate)
		
		if hitRate < 30 {
			fmt.Printf("⚠️  命中率过低，可能存在缓存键问题\n")
		} else if hitRate > 50 {
			fmt.Printf("✅ 缓存系统工作正常\n")
		}
	}
}