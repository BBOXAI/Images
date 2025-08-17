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

const TEST_CACHE_LEVELS_BASE_URL = "http://localhost:8083"

// 创建测试图片
func createImage(text string) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	
	// 随机背景色
	bgColor := color.RGBA{
		uint8(100 + len(text)*10%155),
		uint8(150 + len(text)*20%105),
		uint8(200 - len(text)*15%55),
		255,
	}
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)
	
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

// 上传图片
func uploadImage(data []byte, filename string) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	
	part, err := writer.CreateFormFile("images", filename)
	if err != nil {
		return "", err
	}
	
	part.Write(data)
	writer.Close()
	
	req, err := http.NewRequest("POST", TEST_CACHE_LEVELS_BASE_URL+"/api/upload", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	
	if urls, ok := result["urls"].([]interface{}); ok && len(urls) > 0 {
		return urls[0].(string), nil
	}
	
	return "", fmt.Errorf("no URL returned")
}

// 获取图片并显示缓存信息
func getImageWithCacheInfo(path string) {
	resp, err := http.Get(TEST_CACHE_LEVELS_BASE_URL + path)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	
	fmt.Printf("  Status: %d\n", resp.StatusCode)
	fmt.Printf("  Size: %d bytes\n", len(body))
	fmt.Printf("  Cache-Level: %s\n", resp.Header.Get("X-Cache-Level"))
	fmt.Printf("  Cache-Status: %s\n", resp.Header.Get("X-Cache-Status"))
	fmt.Printf("  Storage-ID: %s\n", resp.Header.Get("X-Storage-ID"))
	fmt.Printf("  Content-Type: %s\n", resp.Header.Get("Content-Type"))
}

func main_test_cache_levels() {
	fmt.Println("===================================")
	fmt.Println("缓存层级测试演示")
	fmt.Println("===================================\n")
	
	// 1. 上传新图片
	fmt.Println("1. 上传新图片...")
	imgData := createImage(fmt.Sprintf("Test-%d", time.Now().Unix()))
	url, err := uploadImage(imgData, "cache_test.png")
	if err != nil {
		fmt.Printf("上传失败: %v\n", err)
		return
	}
	fmt.Printf("上传成功: %s\n\n", url)
	
	// 2. 第一次获取（从Local层）
	fmt.Println("2. 第一次获取（预期从Local层）:")
	getImageWithCacheInfo(url)
	
	// 等待一下让缓存生效
	time.Sleep(100 * time.Millisecond)
	
	// 3. 第二次获取（从Memory层）
	fmt.Println("\n3. 第二次获取（预期从Memory层）:")
	getImageWithCacheInfo(url)
	
	// 4. 第三次获取（确认Memory层）
	fmt.Println("\n4. 第三次获取（确认从Memory层）:")
	getImageWithCacheInfo(url)
	
	// 5. 测试WebP转换
	fmt.Println("\n5. 获取WebP格式（带缓存信息）:")
	getImageWithCacheInfo(url + "?format=webp")
	
	// 6. 测试已存在的图片
	fmt.Println("\n6. 获取已存在的图片:")
	existingURL := "/storage/c09cc20fac2781ba31542cd1116939d40398d7ab.jpg"
	getImageWithCacheInfo(existingURL)
	
	fmt.Println("\n===================================")
	fmt.Println("缓存层级演示完成！")
	fmt.Println("===================================")
	
	fmt.Println("\n说明:")
	fmt.Println("- Memory: 内存缓存（最快）")
	fmt.Println("- Local: 本地磁盘存储")
	fmt.Println("- IOBackend: 远程存储（如果启用）")
	fmt.Println("\n缓存策略:")
	fmt.Println("- 第一次访问从最持久层读取")
	fmt.Println("- 自动缓存到更快的层")
	fmt.Println("- 后续访问从最快层返回")
}