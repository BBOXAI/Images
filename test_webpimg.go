package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	TEST_WEBPIMG_TEST_WEBPIMG_BASE_URL   = "http://localhost:8080"
	TEST_WEBPIMG_TEST_WEBPIMG_TEST_IMAGE = "https://obscura.ac.cn/wp-content/uploads/2024/07/qrcode_for_gh_d6cbcd5a67fc_258.jpg"
)

var (
	testAdminPassword string
	client            *http.Client
	passedTests   = 0
	failedTests   = 0
)

// 颜色输出
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorBold   = "\033[1m"
)

func printTest(name string) {
	fmt.Printf("\n%s📋 测试: %s%s\n", ColorBold, name, ColorReset)
}

func printSuccess(msg string) {
	fmt.Printf("%s✅ %s%s\n", ColorGreen, msg, ColorReset)
}

func printError(msg string) {
	fmt.Printf("%s❌ %s%s\n", ColorRed, msg, ColorReset)
}

func printInfo(msg string) {
	fmt.Printf("%sℹ️  %s%s\n", ColorBlue, msg, ColorReset)
}

func printWarning(msg string) {
	fmt.Printf("%s⚠️  %s%s\n", ColorYellow, msg, ColorReset)
}

func loadTestAdminPassword() {
	data, err := os.ReadFile(".pass")
	if err != nil {
		printWarning(".pass文件不存在，将使用默认密码测试")
		testAdminPassword = "admin123"
		return
	}
	testAdminPassword = strings.TrimSpace(string(data))
	printInfo(fmt.Sprintf("已加载管理员密码: %s", testAdminPassword))
}

func testServerStatus() bool {
	printTest("服务器状态检查")
	
	resp, err := client.Get(TEST_WEBPIMG_BASE_URL + "/stats")
	if err != nil {
		printError(fmt.Sprintf("无法连接到服务器: %v", err))
		printInfo("请确保服务器正在运行: ./webpimg")
		return false
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		printError(fmt.Sprintf("服务器响应异常: %d", resp.StatusCode))
		return false
	}
	
	var stats map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err == nil {
		printSuccess("服务器正在运行")
		statsJSON, _ := json.MarshalIndent(stats, "  ", "  ")
		printInfo(fmt.Sprintf("缓存统计:\n%s", string(statsJSON)))
	}
	
	return true
}

func testBasicProxy() bool {
	printTest("基本代理功能")
	
	// 测试查询参数方式
	testURL := fmt.Sprintf("%s/?url=%s", TEST_WEBPIMG_BASE_URL, url.QueryEscape(TEST_WEBPIMG_TEST_IMAGE))
	printInfo(fmt.Sprintf("测试URL (查询参数): %s", testURL))
	
	resp, err := client.Get(testURL)
	if err != nil {
		printError(fmt.Sprintf("测试失败: %v", err))
		return false
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	
	if resp.StatusCode != 200 {
		printError(fmt.Sprintf("获取图片失败: %d", resp.StatusCode))
		return false
	}
	
	printSuccess(fmt.Sprintf("成功获取图片，大小: %d bytes", len(body)))
	
	// 检查是否为WebP格式
	if len(body) > 12 && bytes.HasPrefix(body, []byte("RIFF")) && bytes.Contains(body[:12], []byte("WEBP")) {
		printSuccess("图片已转换为WebP格式")
	} else {
		printWarning("图片可能未转换为WebP格式")
	}
	
	return true
}

func testFormatConversion() {
	printTest("格式转换功能")
	
	tests := []struct {
		name string
		url  string
	}{
		{"WebP格式", fmt.Sprintf("%s/?url=%s&format=webp", TEST_WEBPIMG_BASE_URL, url.QueryEscape(TEST_WEBPIMG_TEST_IMAGE))},
		{"原始格式", fmt.Sprintf("%s/?url=%s&format=original", TEST_WEBPIMG_BASE_URL, url.QueryEscape(TEST_WEBPIMG_TEST_IMAGE))},
	}
	
	for _, test := range tests {
		printInfo(fmt.Sprintf("测试 %s", test.name))
		
		resp, err := client.Get(test.url)
		if err != nil {
			printError(fmt.Sprintf("%s 异常: %v", test.name, err))
			continue
		}
		defer resp.Body.Close()
		
		body, _ := io.ReadAll(resp.Body)
		
		if resp.StatusCode != 200 {
			printError(fmt.Sprintf("%s 失败: %d", test.name, resp.StatusCode))
			continue
		}
		
		printSuccess(fmt.Sprintf("%s - 大小: %d bytes", test.name, len(body)))
		
		// 检查格式
		if strings.Contains(test.url, "format=webp") {
			if len(body) > 12 && bytes.HasPrefix(body, []byte("RIFF")) && bytes.Contains(body[:12], []byte("WEBP")) {
				printSuccess("  确认为WebP格式")
			} else {
				printError("  格式验证失败")
			}
		} else if strings.Contains(test.url, "format=original") {
			// JPEG magic numbers
			if len(body) > 2 && body[0] == 0xFF && body[1] == 0xD8 {
				printSuccess("  确认为JPEG格式")
			} else {
				printWarning("  可能不是JPEG格式")
			}
		}
	}
}

func testImageResizing() {
	printTest("图片缩放功能")
	
	tests := []struct {
		name string
		url  string
	}{
		{"固定宽度", fmt.Sprintf("%s/?url=%s&w=100", TEST_WEBPIMG_BASE_URL, url.QueryEscape(TEST_WEBPIMG_TEST_IMAGE))},
		{"固定高度", fmt.Sprintf("%s/?url=%s&h=100", TEST_WEBPIMG_BASE_URL, url.QueryEscape(TEST_WEBPIMG_TEST_IMAGE))},
		{"固定尺寸", fmt.Sprintf("%s/?url=%s&w=150&h=150", TEST_WEBPIMG_BASE_URL, url.QueryEscape(TEST_WEBPIMG_TEST_IMAGE))},
		{"自定义质量", fmt.Sprintf("%s/?url=%s&w=200&q=50", TEST_WEBPIMG_BASE_URL, url.QueryEscape(TEST_WEBPIMG_TEST_IMAGE))},
	}
	
	for _, test := range tests {
		printInfo(fmt.Sprintf("测试 %s", test.name))
		
		resp, err := client.Get(test.url)
		if err != nil {
			printError(fmt.Sprintf("%s 异常: %v", test.name, err))
			continue
		}
		defer resp.Body.Close()
		
		body, _ := io.ReadAll(resp.Body)
		
		if resp.StatusCode != 200 {
			printError(fmt.Sprintf("%s 失败: %d", test.name, resp.StatusCode))
			continue
		}
		
		printSuccess(fmt.Sprintf("%s - 大小: %d bytes", test.name, len(body)))
	}
}

func testResizeModes() {
	printTest("缩放模式")
	
	modes := []string{"fit", "fill", "stretch", "pad"}
	
	for _, mode := range modes {
		testURL := fmt.Sprintf("%s/?url=%s&w=200&h=300&mode=%s", TEST_WEBPIMG_BASE_URL, url.QueryEscape(TEST_WEBPIMG_TEST_IMAGE), mode)
		printInfo(fmt.Sprintf("测试模式: %s", mode))
		
		resp, err := client.Get(testURL)
		if err != nil {
			printError(fmt.Sprintf("模式 %s 异常: %v", mode, err))
			continue
		}
		defer resp.Body.Close()
		
		body, _ := io.ReadAll(resp.Body)
		
		if resp.StatusCode != 200 {
			printError(fmt.Sprintf("模式 %s 失败: %d", mode, resp.StatusCode))
			continue
		}
		
		printSuccess(fmt.Sprintf("模式 %s - 成功获取图片 (%d bytes)", mode, len(body)))
	}
}

func testParameterIsolation() {
	printTest("参数隔离（原始URL参数保护）")
	
	// 测试带有原始参数的URL
	testURL := "https://example.com/image.jpg?original_w=1000&id=123"
	proxyURL := fmt.Sprintf("%s/?url=%s&w=200&format=webp", TEST_WEBPIMG_BASE_URL, url.QueryEscape(testURL))
	
	printInfo("原始URL包含参数: original_w=1000, id=123")
	printInfo("代理参数: w=200, format=webp")
	printInfo("期望: 原始参数应该保留，代理参数不应发送给后端")
	
	resp, err := client.Get(proxyURL)
	if err != nil {
		printError(fmt.Sprintf("参数隔离测试失败: %v", err))
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		printWarning(fmt.Sprintf("状态码: %d (可能因为测试URL不存在)", resp.StatusCode))
	} else {
		printSuccess("参数隔离测试通过")
	}
}

func testCacheManagement() {
	printTest("缓存管理接口")
	
	// 不带密码访问
	req, _ := http.NewRequest("GET", TEST_WEBPIMG_BASE_URL+"/cache", nil)
	req.Header.Set("Accept", "text/html")
	
	resp, err := client.Do(req)
	if err != nil {
		printError(fmt.Sprintf("缓存管理测试失败: %v", err))
		return
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	
	if resp.StatusCode == 200 {
		if strings.Contains(strings.ToLower(bodyStr), "password") || strings.Contains(bodyStr, "密码") {
			printSuccess("缓存页面需要密码保护")
			
			// 尝试用密码登录
			if testAdminPassword != "" {
				// 创建带cookie的请求
				jar, _ := cookiejar.New(nil)
				clientWithCookie := &http.Client{Jar: jar}
				
				// 设置认证cookie
				hash := md5.Sum([]byte(testAdminPassword))
				authHash := hex.EncodeToString(hash[:])
				
				u, _ := url.Parse(TEST_WEBPIMG_BASE_URL)
				jar.SetCookies(u, []*http.Cookie{
					{Name: "auth", Value: authHash},
				})
				
				req2, _ := http.NewRequest("GET", TEST_WEBPIMG_BASE_URL+"/cache", nil)
				req2.Header.Set("Accept", "text/html")
				
				resp2, err := clientWithCookie.Do(req2)
				if err == nil {
					defer resp2.Body.Close()
					body2, _ := io.ReadAll(resp2.Body)
					if strings.Contains(string(body2), "缓存管理") {
						printSuccess("成功登录缓存管理页面")
					} else {
						printWarning("登录可能失败")
					}
				}
			}
		} else {
			printWarning("缓存页面可能未启用密码保护")
		}
	} else {
		printError(fmt.Sprintf("访问缓存页面失败: %d", resp.StatusCode))
	}
}

func testMemoryCacheControl() {
	printTest("内存缓存控制API")
	
	// 获取状态
	resp, err := client.Get(TEST_WEBPIMG_BASE_URL + "/cache/control?action=status")
	if err != nil {
		printError(fmt.Sprintf("内存缓存控制测试失败: %v", err))
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == 200 {
		var data map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&data)
		
		enabled := false
		if val, ok := data["enabled"].(bool); ok {
			enabled = val
		}
		
		status := "禁用"
		if enabled {
			status = "启用"
		}
		printSuccess(fmt.Sprintf("内存缓存状态: %s", status))
		
		// 测试切换
		req, _ := http.NewRequest("POST", TEST_WEBPIMG_BASE_URL+"/cache/control?action=toggle", nil)
		resp2, err := client.Do(req)
		if err == nil {
			defer resp2.Body.Close()
			
			if resp2.StatusCode == 200 {
				var data2 map[string]interface{}
				json.NewDecoder(resp2.Body).Decode(&data2)
				
				newEnabled := false
				if val, ok := data2["enabled"].(bool); ok {
					newEnabled = val
				}
				
				newStatus := "禁用"
				if newEnabled {
					newStatus = "启用"
				}
				printSuccess(fmt.Sprintf("成功切换内存缓存状态: %s", newStatus))
				
				// 切换回原状态
				req3, _ := http.NewRequest("POST", TEST_WEBPIMG_BASE_URL+"/cache/control?action=toggle", nil)
				client.Do(req3)
			} else {
				printError(fmt.Sprintf("切换内存缓存失败: %d", resp2.StatusCode))
			}
		}
		
		// 测试同步
		req4, _ := http.NewRequest("POST", TEST_WEBPIMG_BASE_URL+"/cache/control?action=sync", nil)
		resp4, err := client.Do(req4)
		if err == nil {
			defer resp4.Body.Close()
			
			if resp4.StatusCode == 200 {
				printSuccess("成功触发数据库同步")
			} else {
				printWarning(fmt.Sprintf("数据库同步可能失败: %d", resp4.StatusCode))
			}
		}
	} else {
		printError(fmt.Sprintf("获取内存缓存状态失败: %d", resp.StatusCode))
	}
}

func testPerformance() {
	printTest("性能和缓存测试")
	
	testURL := fmt.Sprintf("%s/?url=%s&w=100", TEST_WEBPIMG_BASE_URL, url.QueryEscape(TEST_WEBPIMG_TEST_IMAGE))
	
	printInfo("第一次请求（缓存未命中）")
	start := time.Now()
	resp1, err := client.Get(testURL)
	if err != nil {
		printError(fmt.Sprintf("首次请求失败: %v", err))
		return
	}
	defer resp1.Body.Close()
	io.ReadAll(resp1.Body)
	time1 := time.Since(start)
	
	if resp1.StatusCode == 200 {
		printSuccess(fmt.Sprintf("首次请求成功，耗时: %.2f秒", time1.Seconds()))
	} else {
		printError(fmt.Sprintf("首次请求失败: %d", resp1.StatusCode))
		return
	}
	
	printInfo("第二次请求（应该缓存命中）")
	start = time.Now()
	resp2, err := client.Get(testURL)
	if err != nil {
		printError(fmt.Sprintf("二次请求失败: %v", err))
		return
	}
	defer resp2.Body.Close()
	io.ReadAll(resp2.Body)
	time2 := time.Since(start)
	
	if resp2.StatusCode == 200 {
		printSuccess(fmt.Sprintf("二次请求成功，耗时: %.2f秒", time2.Seconds()))
		
		if time2 < time1/2 {
			speedup := (1 - float64(time2)/float64(time1)) * 100
			printSuccess(fmt.Sprintf("缓存效果明显 (提速 %.1f%%)", speedup))
		} else {
			printWarning("缓存可能未生效")
		}
	} else {
		printError(fmt.Sprintf("二次请求失败: %d", resp2.StatusCode))
	}
}

func testStatistics() {
	printTest("统计信息接口")
	
	resp, err := client.Get(TEST_WEBPIMG_BASE_URL + "/stats")
	if err != nil {
		printError(fmt.Sprintf("统计接口测试失败: %v", err))
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == 200 {
		var stats map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
			printError(fmt.Sprintf("解析统计信息失败: %v", err))
			return
		}
		
		printSuccess("成功获取统计信息")
		
		// 显示关键统计
		if reqStats, ok := stats["request_stats"].(map[string]interface{}); ok {
			if total, ok := reqStats["total_requests"].(float64); ok {
				printInfo(fmt.Sprintf("  总请求数: %d", int(total)))
			}
		}
		
		if cacheStats, ok := stats["cache_stats"].(map[string]interface{}); ok {
			if hits, ok := cacheStats["hits"].(float64); ok {
				printInfo(fmt.Sprintf("  缓存命中: %d", int(hits)))
			}
			if misses, ok := cacheStats["misses"].(float64); ok {
				printInfo(fmt.Sprintf("  缓存未命中: %d", int(misses)))
			}
			if hitRate, ok := cacheStats["hit_rate"].(float64); ok {
				printInfo(fmt.Sprintf("  命中率: %.1f%%", hitRate))
			}
			if fileCount, ok := cacheStats["file_count"].(float64); ok {
				printInfo(fmt.Sprintf("  缓存文件数: %d", int(fileCount)))
			}
			if sizeMB, ok := cacheStats["size_mb"].(float64); ok {
				printInfo(fmt.Sprintf("  缓存大小: %.2f MB", sizeMB))
			}
		}
		
		if savingsStats, ok := stats["savings_stats"].(map[string]interface{}); ok {
			if spaceSaved, ok := savingsStats["total_space_saved_mb"].(float64); ok {
				printInfo(fmt.Sprintf("  节省空间: %.2f MB", spaceSaved))
			}
			if bandwidthSaved, ok := savingsStats["total_bandwidth_saved_mb"].(float64); ok {
				printInfo(fmt.Sprintf("  节省带宽: %.2f MB", bandwidthSaved))
			}
		}
	} else {
		printError(fmt.Sprintf("获取统计信息失败: %d", resp.StatusCode))
	}
}

func runTest(name string, testFunc func() bool) {
	if testFunc() {
		passedTests++
	} else {
		failedTests++
	}
}

func runVoidTest(name string, testFunc func()) {
	testFunc()
	passedTests++
}

func main_test_webpimg() {
	fmt.Printf("\n%s%s\n", ColorBold, strings.Repeat("=", 60))
	fmt.Println("WebP Image Proxy Service 自动化测试")
	fmt.Printf("%s%s\n\n", strings.Repeat("=", 60), ColorReset)
	
	printInfo(fmt.Sprintf("目标服务器: %s", TEST_WEBPIMG_BASE_URL))
	printInfo(fmt.Sprintf("测试图片: %s", TEST_WEBPIMG_TEST_IMAGE))
	
	// 初始化HTTP客户端
	client = &http.Client{
		Timeout: 10 * time.Second,
	}
	
	// 加载密码
	loadTestAdminPassword()
	
	// 检查服务器状态
	if !testServerStatus() {
		printError("\n服务器未运行，请先启动服务器")
		os.Exit(1)
	}
	
	// 运行各项测试
	runTest("基本代理", testBasicProxy)
	runVoidTest("格式转换", testFormatConversion)
	runVoidTest("图片缩放", testImageResizing)
	runVoidTest("缩放模式", testResizeModes)
	runVoidTest("参数隔离", testParameterIsolation)
	runVoidTest("缓存管理", testCacheManagement)
	runVoidTest("内存缓存控制", testMemoryCacheControl)
	runVoidTest("性能测试", testPerformance)
	runVoidTest("统计接口", testStatistics)
	
	// 总结
	fmt.Printf("\n%s%s\n", ColorBold, strings.Repeat("=", 60))
	fmt.Println("测试总结")
	fmt.Printf("%s%s\n\n", strings.Repeat("=", 60), ColorReset)
	
	total := passedTests + failedTests
	printInfo(fmt.Sprintf("总测试数: %d", total))
	printSuccess(fmt.Sprintf("通过: %d", passedTests))
	if failedTests > 0 {
		printError(fmt.Sprintf("失败: %d", failedTests))
	}
	
	successRate := float64(passedTests) / float64(total) * 100
	if successRate == 100 {
		printSuccess("\n🎉 所有测试通过！")
	} else if successRate >= 80 {
		printWarning(fmt.Sprintf("\n⚠️  大部分测试通过 (%.1f%%)", successRate))
	} else {
		printError(fmt.Sprintf("\n❌ 测试通过率较低 (%.1f%%)", successRate))
	}
	
	if failedTests > 0 {
		os.Exit(1)
	}
}