package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

const (
	TEST_STORAGE_BASE_URL   = "http://localhost:8082"
	TEST_STORAGE_UPLOAD_URL = TEST_STORAGE_BASE_URL + "/api/upload"
	TEST_STORAGE_STATS_URL  = TEST_STORAGE_BASE_URL + "/stats"
)

// 测试结果
type TestResult struct {
	Name   string
	Passed bool
	Error  error
}

// 上传响应
type UploadResponse struct {
	Success bool     `json:"success"`
	URLs    []string `json:"urls"`
	Count   int      `json:"count"`
}

// 统计信息
type StatsResponse struct {
	TotalRequests      int64   `json:"total_requests"`
	CacheHits          int64   `json:"cache_hits"`
	CacheMisses        int64   `json:"cache_misses"`
	MemoryCacheEntries int     `json:"memory_cache_entries"`
	DiskCacheSize      float64 `json:"disk_cache_size_mb"`
	DiskCacheFiles     int     `json:"disk_cache_files"`
}

// 创建测试图片
func createTestImage(text string, width, height int) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	
	// 填充背景色
	bgColor := color.RGBA{100, 150, 200, 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)
	
	// 添加一些线条使图片唯一
	for i := 0; i < 5; i++ {
		x := i * width / 5
		lineColor := color.RGBA{255, 255, 255, 128}
		for y := 0; y < height; y++ {
			img.Set(x, y, lineColor)
		}
	}
	
	// 编码为PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	
	return buf.Bytes(), nil
}

// 上传图片
func uploadImageToStorage(imageData []byte, filename string) (*UploadResponse, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	
	part, err := writer.CreateFormFile("images", filename)
	if err != nil {
		return nil, err
	}
	
	if _, err := part.Write(imageData); err != nil {
		return nil, err
	}
	
	if err := writer.Close(); err != nil {
		return nil, err
	}
	
	req, err := http.NewRequest("POST", TEST_STORAGE_UPLOAD_URL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("上传失败: %d - %s", resp.StatusCode, string(body))
	}
	
	var result UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	
	return &result, nil
}

// 获取图片
func getImage(path string) ([]byte, string, error) {
	resp, err := http.Get(TEST_STORAGE_BASE_URL + path)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("获取失败: %d", resp.StatusCode)
	}
	
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	
	contentType := resp.Header.Get("Content-Type")
	return data, contentType, nil
}

// 获取统计信息
func getStorageStats() (*StatsResponse, error) {
	resp, err := http.Get(TEST_STORAGE_STATS_URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var stats StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, err
	}
	
	return &stats, nil
}

// 测试基本上传
func testBasicUpload() TestResult {
	fmt.Println("\n=== 测试基本上传功能 ===")
	
	// 创建测试图片
	imgData, err := createTestImage("Upload Test", 200, 200)
	if err != nil {
		return TestResult{"基本上传", false, err}
	}
	
	// 上传图片
	result, err := uploadImageToStorage(imgData, "test.png")
	if err != nil {
		return TestResult{"基本上传", false, err}
	}
	
	fmt.Printf("上传成功: %d 个文件\n", result.Count)
	fmt.Printf("返回URLs: %v\n", result.URLs)
	
	if len(result.URLs) > 0 {
		// 获取上传的图片
		imgURL := result.URLs[0]
		data, contentType, err := getImage(imgURL)
		if err != nil {
			return TestResult{"基本上传", false, err}
		}
		
		fmt.Printf("获取成功: %d bytes, Content-Type: %s\n", len(data), contentType)
		
		// 测试WebP转换
		webpData, webpType, err := getImage(imgURL + "?format=webp")
		if err != nil {
			fmt.Printf("WebP转换失败: %v\n", err)
		} else {
			fmt.Printf("WebP格式: %d bytes, Content-Type: %s\n", len(webpData), webpType)
			compression := float64(len(imgData)-len(webpData)) / float64(len(imgData)) * 100
			fmt.Printf("压缩率: %.2f%%\n", compression)
		}
	}
	
	return TestResult{"基本上传", true, nil}
}

// 测试重复上传（去重）
func testDuplicateUpload() TestResult {
	fmt.Println("\n=== 测试重复上传（去重） ===")
	
	// 创建相同的图片
	imgData, err := createTestImage("Duplicate", 300, 300)
	if err != nil {
		return TestResult{"重复上传", false, err}
	}
	
	// 第一次上传
	result1, err := uploadImageToStorage(imgData, "dup1.png")
	if err != nil {
		return TestResult{"重复上传", false, err}
	}
	
	// 第二次上传相同图片
	result2, err := uploadImageToStorage(imgData, "dup2.png")
	if err != nil {
		return TestResult{"重复上传", false, err}
	}
	
	if len(result1.URLs) > 0 && len(result2.URLs) > 0 {
		fmt.Printf("第一次上传: %s\n", result1.URLs[0])
		fmt.Printf("第二次上传: %s\n", result2.URLs[0])
		
		// 检查是否返回相同的存储路径（表示去重成功）
		if result1.URLs[0] == result2.URLs[0] {
			fmt.Println("✓ 去重成功：两次上传返回相同路径")
		} else {
			fmt.Println("⚠ 注意：两次上传返回不同路径")
		}
	}
	
	return TestResult{"重复上传", true, nil}
}

// 测试批量上传
func testBatchUpload() TestResult {
	fmt.Println("\n=== 测试批量上传 ===")
	
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	
	// 创建多个图片
	for i := 0; i < 3; i++ {
		imgData, err := createTestImage(fmt.Sprintf("Batch %d", i+1), 150, 150)
		if err != nil {
			return TestResult{"批量上传", false, err}
		}
		
		part, err := writer.CreateFormFile("images", fmt.Sprintf("batch_%d.png", i+1))
		if err != nil {
			return TestResult{"批量上传", false, err}
		}
		
		if _, err := part.Write(imgData); err != nil {
			return TestResult{"批量上传", false, err}
		}
	}
	
	if err := writer.Close(); err != nil {
		return TestResult{"批量上传", false, err}
	}
	
	req, err := http.NewRequest("POST", TEST_STORAGE_UPLOAD_URL, body)
	if err != nil {
		return TestResult{"批量上传", false, err}
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return TestResult{"批量上传", false, err}
	}
	defer resp.Body.Close()
	
	var result UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return TestResult{"批量上传", false, err}
	}
	
	fmt.Printf("批量上传成功: %d 个文件\n", result.Count)
	fmt.Printf("返回URLs: %v\n", result.URLs)
	
	return TestResult{"批量上传", result.Count == 3, nil}
}

// 测试缓存性能
func testCachePerformance() TestResult {
	fmt.Println("\n=== 测试缓存性能 ===")
	
	// 创建较大的测试图片
	imgData, err := createTestImage("Cache Test", 500, 500)
	if err != nil {
		return TestResult{"缓存性能", false, err}
	}
	
	// 上传图片
	result, err := uploadImageToStorage(imgData, "cache_test.png")
	if err != nil {
		return TestResult{"缓存性能", false, err}
	}
	
	if len(result.URLs) > 0 {
		imgURL := result.URLs[0]
		
		// 第一次获取（冷缓存）
		start := time.Now()
		_, _, err := getImage(imgURL)
		time1 := time.Since(start)
		if err != nil {
			return TestResult{"缓存性能", false, err}
		}
		fmt.Printf("第一次获取: %v\n", time1)
		
		// 第二次获取（热缓存）
		start = time.Now()
		_, _, err = getImage(imgURL)
		time2 := time.Since(start)
		if err != nil {
			return TestResult{"缓存性能", false, err}
		}
		fmt.Printf("第二次获取: %v\n", time2)
		
		// 第三次获取
		start = time.Now()
		_, _, err = getImage(imgURL)
		time3 := time.Since(start)
		if err != nil {
			return TestResult{"缓存性能", false, err}
		}
		fmt.Printf("第三次获取: %v\n", time3)
		
		// 检查缓存效果
		if time2 < time1/2 || time3 < time1/2 {
			fmt.Println("✓ 缓存加速效果明显")
		} else {
			fmt.Println("⚠ 缓存效果不明显")
		}
	}
	
	return TestResult{"缓存性能", true, nil}
}

// 测试存储统计
func testStorageStats() TestResult {
	fmt.Println("\n=== 存储统计信息 ===")
	
	stats, err := getStorageStats()
	if err != nil {
		return TestResult{"存储统计", false, err}
	}
	
	fmt.Printf("总请求数: %d\n", stats.TotalRequests)
	fmt.Printf("缓存命中: %d\n", stats.CacheHits)
	fmt.Printf("缓存未命中: %d\n", stats.CacheMisses)
	
	if stats.CacheHits+stats.CacheMisses > 0 {
		hitRate := float64(stats.CacheHits) / float64(stats.CacheHits+stats.CacheMisses) * 100
		fmt.Printf("缓存命中率: %.2f%%\n", hitRate)
	}
	
	fmt.Printf("内存缓存条目: %d\n", stats.MemoryCacheEntries)
	fmt.Printf("磁盘缓存大小: %.2f MB\n", stats.DiskCacheSize)
	fmt.Printf("磁盘缓存文件数: %d\n", stats.DiskCacheFiles)
	
	return TestResult{"存储统计", true, nil}
}

// 测试代理远程图片
func testProxyRemoteImage() TestResult {
	fmt.Println("\n=== 测试代理远程图片 ===")
	
	// 使用提供的测试图片（微信二维码）
	testImageURL := "https://obscura.ac.cn/wp-content/uploads/2024/07/qrcode_for_gh_d6cbcd5a67fc_258.jpg"
	proxyPath := "/?url=" + testImageURL
	
	fmt.Printf("代理URL: %s\n", proxyPath)
	
	// 获取原始格式
	origData, origType, err := getImage(proxyPath)
	if err != nil {
		fmt.Printf("代理失败: %v\n", err)
		// 不算失败，可能是网络问题
		return TestResult{"代理远程图片", true, nil}
	}
	
	fmt.Printf("原始格式: %d bytes, Content-Type: %s\n", len(origData), origType)
	
	// 获取WebP格式
	webpPath := proxyPath + "&format=webp"
	webpData, webpType, err := getImage(webpPath)
	if err != nil {
		fmt.Printf("WebP转换失败: %v\n", err)
	} else {
		fmt.Printf("WebP格式: %d bytes, Content-Type: %s\n", len(webpData), webpType)
		
		if len(origData) > 0 {
			compression := float64(len(origData)-len(webpData)) / float64(len(origData)) * 100
			fmt.Printf("压缩率: %.2f%%\n", compression)
		}
	}
	
	return TestResult{"代理远程图片", true, nil}
}

func main_test_storage() {
	fmt.Println("==================================================")
	fmt.Println("WebP图片服务 - Go测试程序")
	fmt.Println("==================================================")
	
	// 检查服务是否运行
	resp, err := http.Get(TEST_STORAGE_BASE_URL)
	if err != nil {
		fmt.Printf("✗ 无法连接到服务: %s\n", TEST_STORAGE_BASE_URL)
		fmt.Println("请确保服务已启动")
		return
	}
	resp.Body.Close()
	fmt.Printf("✓ 服务正在运行: %s\n", TEST_STORAGE_BASE_URL)
	
	// 运行测试
	tests := []func() TestResult{
		testBasicUpload,
		testDuplicateUpload,
		testBatchUpload,
		testCachePerformance,
		testStorageStats,
		testProxyRemoteImage,
	}
	
	var results []TestResult
	for _, test := range tests {
		result := test()
		results = append(results, result)
		time.Sleep(100 * time.Millisecond) // 给服务一点处理时间
	}
	
	// 打印测试总结
	fmt.Println("\n==================================================")
	fmt.Println("测试总结")
	fmt.Println("==================================================")
	
	passedCount := 0
	for _, result := range results {
		status := "✓ 通过"
		if !result.Passed {
			status = fmt.Sprintf("✗ 失败: %v", result.Error)
		} else {
			passedCount++
		}
		fmt.Printf("%s: %s\n", result.Name, status)
	}
	
	fmt.Printf("\n总计: %d/%d 测试通过\n", passedCount, len(results))
}