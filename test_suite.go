package main

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type TestCaseResult struct {
	Name     string
	Passed   bool
	Message  string
	Duration time.Duration
}

type TestSuite struct {
	BaseURL string
	Results []TestCaseResult
	Client  *http.Client
}

func NewTestSuite(baseURL string) *TestSuite {
	return &TestSuite{
		BaseURL: baseURL,
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
		Results: []TestCaseResult{},
	}
}

func (ts *TestSuite) Run() {
	fmt.Println("=== WebP Image Proxy Test Suite ===\n")
	
	// Basic connectivity tests
	ts.TestHealthCheck()
	ts.TestStatsAPI()
	ts.TestCachePage()
	
	// Image proxy tests
	ts.TestImageProxy()
	ts.TestWebPConversion()
	ts.TestImageResize()
	ts.TestImageQuality()
	
	// Cache tests
	ts.TestCacheHit()
	ts.TestCacheExpiry()
	
	// Print results
	ts.PrintResults()
}

func (ts *TestSuite) TestHealthCheck() {
	start := time.Now()
	resp, err := ts.Client.Get(ts.BaseURL + "/stats")
	duration := time.Since(start)
	
	if err != nil {
		ts.Results = append(ts.Results, TestCaseResult{
			Name:     "Health Check",
			Passed:   false,
			Message:  fmt.Sprintf("Failed to connect: %v", err),
			Duration: duration,
		})
		return
	}
	defer resp.Body.Close()
	
	ts.Results = append(ts.Results, TestCaseResult{
		Name:     "Health Check",
		Passed:   resp.StatusCode == 200,
		Message:  fmt.Sprintf("Status code: %d", resp.StatusCode),
		Duration: duration,
	})
}

func (ts *TestSuite) TestStatsAPI() {
	start := time.Now()
	resp, err := ts.Client.Get(ts.BaseURL + "/stats")
	duration := time.Since(start)
	
	if err != nil {
		ts.Results = append(ts.Results, TestCaseResult{
			Name:     "Stats API",
			Passed:   false,
			Message:  fmt.Sprintf("Request failed: %v", err),
			Duration: duration,
		})
		return
	}
	defer resp.Body.Close()
	
	var stats map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&stats)
	
	passed := err == nil && stats["cache_stats"] != nil
	message := "Valid JSON response"
	if !passed {
		message = fmt.Sprintf("Invalid response: %v", err)
	}
	
	ts.Results = append(ts.Results, TestCaseResult{
		Name:     "Stats API",
		Passed:   passed,
		Message:  message,
		Duration: duration,
	})
}

func (ts *TestSuite) TestCachePage() {
	start := time.Now()
	resp, err := ts.Client.Get(ts.BaseURL + "/cache")
	duration := time.Since(start)
	
	if err != nil {
		ts.Results = append(ts.Results, TestCaseResult{
			Name:     "Cache Page",
			Passed:   false,
			Message:  fmt.Sprintf("Request failed: %v", err),
			Duration: duration,
		})
		return
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	passed := resp.StatusCode == 200 && strings.Contains(string(body), "缓存管理")
	
	ts.Results = append(ts.Results, TestCaseResult{
		Name:     "Cache Page",
		Passed:   passed,
		Message:  fmt.Sprintf("Status: %d, Contains UI: %v", resp.StatusCode, passed),
		Duration: duration,
	})
}

func (ts *TestSuite) TestImageProxy() {
	testURL := "https://via.placeholder.com/100"
	start := time.Now()
	resp, err := ts.Client.Get(ts.BaseURL + "/" + testURL)
	duration := time.Since(start)
	
	if err != nil {
		ts.Results = append(ts.Results, TestCaseResult{
			Name:     "Image Proxy",
			Passed:   false,
			Message:  fmt.Sprintf("Request failed: %v", err),
			Duration: duration,
		})
		return
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	passed := resp.StatusCode == 200 && len(body) > 0
	
	ts.Results = append(ts.Results, TestCaseResult{
		Name:     "Image Proxy",
		Passed:   passed,
		Message:  fmt.Sprintf("Status: %d, Size: %d bytes", resp.StatusCode, len(body)),
		Duration: duration,
	})
}

func (ts *TestSuite) TestWebPConversion() {
	testURL := "https://via.placeholder.com/100"
	start := time.Now()
	
	req, _ := http.NewRequest("GET", ts.BaseURL+"/"+testURL, nil)
	req.Header.Set("Accept", "image/webp")
	
	resp, err := ts.Client.Do(req)
	duration := time.Since(start)
	
	if err != nil {
		ts.Results = append(ts.Results, TestCaseResult{
			Name:     "WebP Conversion",
			Passed:   false,
			Message:  fmt.Sprintf("Request failed: %v", err),
			Duration: duration,
		})
		return
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	// Check for WebP magic bytes: RIFF....WEBP
	isWebP := len(body) > 12 && 
		string(body[0:4]) == "RIFF" && 
		string(body[8:12]) == "WEBP"
	
	ts.Results = append(ts.Results, TestCaseResult{
		Name:     "WebP Conversion",
		Passed:   isWebP,
		Message:  fmt.Sprintf("Content-Type: %s, Is WebP: %v", resp.Header.Get("Content-Type"), isWebP),
		Duration: duration,
	})
}

func (ts *TestSuite) TestImageResize() {
	testURL := "https://via.placeholder.com/200"
	start := time.Now()
	resp, err := ts.Client.Get(ts.BaseURL + "/" + testURL + "?w=50")
	duration := time.Since(start)
	
	if err != nil {
		ts.Results = append(ts.Results, TestCaseResult{
			Name:     "Image Resize",
			Passed:   false,
			Message:  fmt.Sprintf("Request failed: %v", err),
			Duration: duration,
		})
		return
	}
	defer resp.Body.Close()
	
	img, _, err := image.Decode(resp.Body)
	passed := err == nil && img != nil
	message := "Image decoded successfully"
	if passed {
		bounds := img.Bounds()
		width := bounds.Dx()
		message = fmt.Sprintf("Resized to width: %d", width)
		passed = width <= 50
	}
	
	ts.Results = append(ts.Results, TestCaseResult{
		Name:     "Image Resize",
		Passed:   passed,
		Message:  message,
		Duration: duration,
	})
}

func (ts *TestSuite) TestImageQuality() {
	testURL := "https://via.placeholder.com/100"
	
	// Get original size
	resp1, _ := ts.Client.Get(ts.BaseURL + "/" + testURL + "?q=100")
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	
	// Get lower quality
	start := time.Now()
	resp2, _ := ts.Client.Get(ts.BaseURL + "/" + testURL + "?q=50")
	duration := time.Since(start)
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	
	// Lower quality should be smaller
	passed := len(body2) < len(body1)
	
	ts.Results = append(ts.Results, TestCaseResult{
		Name:     "Image Quality",
		Passed:   passed,
		Message:  fmt.Sprintf("Q100: %d bytes, Q50: %d bytes", len(body1), len(body2)),
		Duration: duration,
	})
}

func (ts *TestSuite) TestCacheHit() {
	testURL := "https://via.placeholder.com/100"
	
	// First request
	start1 := time.Now()
	resp1, _ := ts.Client.Get(ts.BaseURL + "/" + testURL)
	duration1 := time.Since(start1)
	resp1.Body.Close()
	
	// Second request (should be cached)
	start2 := time.Now()
	resp2, _ := ts.Client.Get(ts.BaseURL + "/" + testURL)
	duration2 := time.Since(start2)
	resp2.Body.Close()
	
	// Cache hit should be faster
	passed := duration2 < duration1
	
	ts.Results = append(ts.Results, TestCaseResult{
		Name:     "Cache Hit",
		Passed:   passed,
		Message:  fmt.Sprintf("First: %v, Second: %v", duration1, duration2),
		Duration: duration2,
	})
}

func (ts *TestSuite) TestCacheExpiry() {
	// This would need to wait for cache expiry time
	// For now, just check that cache headers are set
	testURL := "https://via.placeholder.com/100"
	start := time.Now()
	resp, _ := ts.Client.Get(ts.BaseURL + "/" + testURL)
	duration := time.Since(start)
	defer resp.Body.Close()
	
	cacheControl := resp.Header.Get("Cache-Control")
	passed := cacheControl != ""
	
	ts.Results = append(ts.Results, TestCaseResult{
		Name:     "Cache Headers",
		Passed:   passed,
		Message:  fmt.Sprintf("Cache-Control: %s", cacheControl),
		Duration: duration,
	})
}

func (ts *TestSuite) PrintResults() {
	fmt.Println("\n=== Test Results ===")
	
	totalTests := len(ts.Results)
	passedTests := 0
	totalDuration := time.Duration(0)
	
	for _, result := range ts.Results {
		status := "❌"
		if result.Passed {
			status = "✅"
			passedTests++
		}
		totalDuration += result.Duration
		
		fmt.Printf("%s %s - %s (%v)\n", status, result.Name, result.Message, result.Duration)
	}
	
	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Total: %d, Passed: %d, Failed: %d\n", totalTests, passedTests, totalTests-passedTests)
	fmt.Printf("Success Rate: %.1f%%\n", float64(passedTests)*100/float64(totalTests))
	fmt.Printf("Total Duration: %v\n", totalDuration)
	
	// Write markdown report
	report := fmt.Sprintf("# Test Report\n\n")
	report += fmt.Sprintf("## Summary\n")
	report += fmt.Sprintf("- **Total Tests**: %d\n", totalTests)
	report += fmt.Sprintf("- **Passed**: %d\n", passedTests)
	report += fmt.Sprintf("- **Failed**: %d\n", totalTests-passedTests)
	report += fmt.Sprintf("- **Success Rate**: %.1f%%\n\n", float64(passedTests)*100/float64(totalTests))
	
	report += "## Test Results\n"
	for _, result := range ts.Results {
		status := "❌"
		if result.Passed {
			status = "✅"
		}
		report += fmt.Sprintf("%s **%s** - %s (%v)\n", status, result.Name, result.Message, result.Duration)
	}
	
	os.WriteFile("test-report.md", []byte(report), 0644)
	
	// Exit with appropriate code
	if passedTests < totalTests {
		os.Exit(1)
	}
}

func main_test() {
	baseURL := "http://localhost:8080"
	if url := os.Getenv("TEST_BASE_URL"); url != "" {
		baseURL = url
	}
	
	suite := NewTestSuite(baseURL)
	suite.Run()
}