package main

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
	"encoding/json"
)

const TEST_CLEANUP_TEST_CLEANUP_BASE_URL = "http://localhost:8080"

func getMemCacheStats() map[string]interface{} {
	resp, err := http.Get(TEST_CLEANUP_BASE_URL + "/stats")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return nil
	}
	defer resp.Body.Close()
	
	var stats map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&stats)
	
	if memCache, ok := stats["memory_cache"].(map[string]interface{}); ok {
		return memCache
	}
	return nil
}

func main_test_cleanup() {
	fmt.Println("=== 内存缓存清理测试 ===")
	fmt.Println("测试策略：创建大量不同参数的缓存，观察清理机制")
	
	// 不同的测试图片URL
	testImages := []string{
		"https://obscura.ac.cn/wp-content/uploads/2024/07/qrcode_for_gh_d6cbcd5a67fc_258.jpg",
		"https://httpbin.org/image/jpeg",
		"https://httpbin.org/image/png",
		"https://httpbin.org/image/webp",
	}
	
	// 初始状态
	fmt.Println("\n初始内存缓存状态:")
	if stats := getMemCacheStats(); stats != nil {
		fmt.Printf("  条目数: %.0f / %.0f\n", stats["entries"], stats["max_entries"])
		fmt.Printf("  大小: %.2f MB / %.2f MB\n", stats["estimated_size_mb"], stats["max_size_mb"])
	}
	
	// 生成大量缓存
	fmt.Println("\n生成测试缓存...")
	for i := 0; i < 50; i++ {
		for _, imgURL := range testImages {
			// 使用不同的参数组合
			variations := []string{
				fmt.Sprintf("?url=%s&w=%d", url.QueryEscape(imgURL), 100+i*10),
				fmt.Sprintf("?url=%s&h=%d", url.QueryEscape(imgURL), 100+i*10),
				fmt.Sprintf("?url=%s&w=%d&h=%d", url.QueryEscape(imgURL), 100+i*5, 100+i*5),
			}
			
			for _, variation := range variations {
				testURL := TEST_CLEANUP_BASE_URL + "/" + variation
				resp, err := http.Get(testURL)
				if err != nil {
					continue
				}
				resp.Body.Close()
			}
		}
		
		// 每10次请求检查一次状态
		if (i+1)%10 == 0 {
			fmt.Printf("已发送 %d 组请求\n", i+1)
			if stats := getMemCacheStats(); stats != nil {
				fmt.Printf("  当前条目数: %.0f / %.0f\n", stats["entries"], stats["max_entries"])
				fmt.Printf("  当前大小: %.2f MB / %.2f MB\n", stats["estimated_size_mb"], stats["max_size_mb"])
			}
		}
		
		time.Sleep(100 * time.Millisecond)
	}
	
	// 最终状态
	fmt.Println("\n最终内存缓存状态:")
	if stats := getMemCacheStats(); stats != nil {
		fmt.Printf("  条目数: %.0f / %.0f\n", stats["entries"], stats["max_entries"])
		fmt.Printf("  大小: %.2f MB / %.2f MB\n", stats["estimated_size_mb"], stats["max_size_mb"])
		fmt.Printf("  清理间隔: %v\n", stats["cleanup_interval"])
		fmt.Printf("  访问窗口: %v\n", stats["access_window"])
	}
	
	// 访问部分缓存，创建访问频率差异
	fmt.Println("\n创建访问频率差异（访问前10个缓存多次）...")
	for j := 0; j < 5; j++ {
		for i := 0; i < 10; i++ {
			imgURL := testImages[0]
			testURL := fmt.Sprintf("%s/?url=%s&w=%d", TEST_CLEANUP_BASE_URL, url.QueryEscape(imgURL), 100+i*10)
			resp, _ := http.Get(testURL)
			if resp != nil {
				resp.Body.Close()
			}
		}
	}
	
	fmt.Println("\n等待清理周期（5分钟）...")
	fmt.Println("提示：可以观察 webpimg.log 查看清理日志")
	
	// 每30秒检查一次状态
	for i := 0; i < 10; i++ {
		time.Sleep(30 * time.Second)
		fmt.Printf("\n[%d分钟后] 内存缓存状态:\n", (i+1)/2)
		if stats := getMemCacheStats(); stats != nil {
			fmt.Printf("  条目数: %.0f / %.0f\n", stats["entries"], stats["max_entries"])
			fmt.Printf("  大小: %.2f MB / %.2f MB\n", stats["estimated_size_mb"], stats["max_size_mb"])
		}
	}
	
	fmt.Println("\n测试完成！")
}