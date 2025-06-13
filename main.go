package main

import (
	"bytes"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HugoSmits86/nativewebp"
	_ "modernc.org/sqlite"
)

var (
	requestCount int64
	cacheDir     = "cache"
	db           *sql.DB
	dbMutex      sync.Mutex
	cacheHits    int64
	cacheMisses  int64
	cacheMutex   sync.RWMutex
	maxCacheSize = int64(100 * 1024 * 1024) // 100MB
	localTZ      *time.Location
)

func main() {
	var err error
	localTZ, err = time.LoadLocation("Asia/Shanghai") // 中国时区
	if err != nil {
		log.Printf("加载时区失败，使用本地时区: %v", err)
		localTZ = time.Local
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Fatalf("创建缓存目录失败: %v", err)
	}

	thumbDir := filepath.Join(cacheDir, "thumbs")
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		log.Fatalf("创建缩略图目录失败: %v", err)
	}

	initDB()

	go cleanExpiredCache()

	http.HandleFunc("/stats", handleStats)
	http.HandleFunc("/cache", handleCacheList)
	http.HandleFunc("/thumb/", handleThumbnail)
	http.HandleFunc("/", handleImageProxy)

	port := "8080"
	fmt.Printf("Address：%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func initDB() {
	var err error
	db, err = sql.Open("sqlite", "./imgproxy.db")
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}

	_, err = db.Exec(`PRAGMA journal_mode = WAL;
		PRAGMA synchronous = NORMAL;
		PRAGMA temp_store = MEMORY;
		PRAGMA busy_timeout = 5000;`)
	if err != nil {
		log.Printf("Setting database parameters failed: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS cache (
		url TEXT PRIMARY KEY,
		file_path TEXT,
		thumb_path TEXT,
		format TEXT,
		access_count INTEGER DEFAULT 1,
		last_access TIMESTAMP,
		created_at TIMESTAMP
	)`)
	if err != nil {
		log.Fatalf("Creating cache table failed: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS stats (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		total_requests INTEGER DEFAULT 0,
		total_cache_hits INTEGER DEFAULT 0,
		total_cache_misses INTEGER DEFAULT 0,
		total_bytes_saved INTEGER DEFAULT 0,
		total_bandwidth_saved INTEGER DEFAULT 0
	)`)
	if err != nil {
		log.Fatalf("Creating stats table failed: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM stats").Scan(&count)
	if err != nil {
		log.Fatalf("Querying stats table failed: %v", err)
	}

	if count == 0 {
		_, err = db.Exec("INSERT INTO stats (total_requests, total_cache_hits, total_cache_misses, total_bytes_saved, total_bandwidth_saved) VALUES (0, 0, 0, 0, 0)")
		if err != nil {
			log.Fatalf("Initializing statistics failed: %v", err)
		}
	}

	err = db.QueryRow("SELECT total_requests FROM stats WHERE id = 1").Scan(&requestCount)
	if err != nil {
		log.Printf("Querying total requests failed: %v，using default value 0", err)
		requestCount = 0
	}
}

func cleanExpiredCache() {
	for {
		time.Sleep(6 * time.Hour) //  Expired cache every 6 hours
		log.Println("Starting to clean expired cache...")

		dbMutex.Lock()
		rows, err := db.Query(`
			SELECT url, file_path, access_count, last_access FROM cache
			WHERE datetime(last_access, 'localtime') < datetime('now', 'localtime', '-10 minutes')
		`)
		if err != nil {
			log.Printf("Querying expired cache failed: %v", err)
			dbMutex.Unlock()
			continue
		}

		var expiredURLs []string
		var expiredFiles []string
		var count int

		for rows.Next() {
			var url, filePath string
			var accessCount int
			var lastAccessStr string
			if err := rows.Scan(&url, &filePath, &accessCount, &lastAccessStr); err != nil {
				log.Printf("Reading cache record failed: %v", err)
				continue
			}

			lastAccess, err := parseDBTime(lastAccessStr)
			if err != nil {
				log.Printf("解析时间失败: %v", err)
				continue
			}

			expireMinutes := 10

			expireTime := lastAccess.Add(time.Duration(expireMinutes) * time.Minute)
			if time.Now().In(localTZ).After(expireTime) {
				expiredURLs = append(expiredURLs, url)
				expiredFiles = append(expiredFiles, filePath)
				count++
			}
		}
		rows.Close()

		for i, filePath := range expiredFiles {
			if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
				log.Printf("Deleting expired cache file failed %s: %v", filePath, err)
			}

			_, err := db.Exec("DELETE FROM cache WHERE url = ?", expiredURLs[i])
			if err != nil {
				log.Printf("Deleting cache record failed: %v", err)
			}
		}

		dbMutex.Unlock()
		log.Printf("Finished deleting %d expired cache files", count)
	}
}

func getCacheFilePath(imageURL string, format string) string {
	hasher := md5.New()
	hasher.Write([]byte(imageURL))
	hash := hex.EncodeToString(hasher.Sum(nil))

	var ext string
	switch format {
	case "png":
		ext = ".png"
	case "gif":
		ext = ".gif"
	default:
		ext = ".jpg"
	}

	return filepath.Join(cacheDir, hash+ext)
}

func updateCacheRecord(imageURL, filePath, thumbPath, format string, isHit bool, originalSize, compressedSize int64) {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	currentTime := formatTimeForDB(time.Now().In(localTZ))

	if isHit {
		_, err := db.Exec(
			"UPDATE cache SET access_count = access_count + 1, last_access = ? WHERE url = ?",
			currentTime, imageURL,
		)
		if err != nil {
			log.Printf("Updating cache record failed: %v", err)
		}

		_, err = db.Exec("UPDATE stats SET total_cache_hits = total_cache_hits + 1 WHERE id = 1")
		if err != nil {
			log.Printf("Updating cache hit statistics failed: %v", err)
		}

		if originalSize > 0 && compressedSize > 0 {
			bytesSaved := originalSize - compressedSize
			if bytesSaved > 0 {
				_, err = db.Exec("UPDATE stats SET total_bytes_saved = total_bytes_saved + ?, total_bandwidth_saved = total_bandwidth_saved + ? WHERE id = 1", bytesSaved, originalSize)
				if err != nil {
					log.Printf("更新节省空间统计失败: %v", err)
				}
			}
		}
	} else {
		_, err := db.Exec(
			"INSERT INTO cache (url, file_path, thumb_path, format, access_count, last_access, created_at) VALUES (?, ?, ?, ?, 1, ?, ?)",
			imageURL, filePath, thumbPath, format, currentTime, currentTime,
		)
		if err != nil {
			log.Printf("Updating cache miss statistics failed: %v", err)
		}

		_, err = db.Exec("UPDATE stats SET total_cache_misses = total_cache_misses + 1 WHERE id = 1")
		if err != nil {
			log.Printf("Updating cache miss statistics failed: %v", err)
		}

		if originalSize > 0 && compressedSize > 0 {
			bytesSaved := originalSize - compressedSize
			if bytesSaved > 0 {
				_, err = db.Exec("UPDATE stats SET total_bytes_saved = total_bytes_saved + ?, total_bandwidth_saved = total_bandwidth_saved + ? WHERE id = 1", bytesSaved, originalSize)
				if err != nil {
					log.Printf("Updating cache miss statistics failed: %v", err)
				}
			}
		}
	}

	_, err := db.Exec("UPDATE stats SET total_requests = ? WHERE id = 1", atomic.LoadInt64(&requestCount))
	if err != nil {
		log.Printf("Updating total requests statistics failed: %v", err)
	}
}

func getFromCache(imageURL string) ([]byte, string, bool) {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	var filePath, format string
	var accessCount int
	err := db.QueryRow(
		"SELECT file_path, format, access_count FROM cache WHERE url = ?",
		imageURL,
	).Scan(&filePath, &format, &accessCount)

	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("Querying cache record failed: %v", err)
		}
		return nil, "", false
	}

	imgData, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("Reading cache file failed: %v", err)
		if os.IsNotExist(err) {
			_, _ = db.Exec("DELETE FROM cache WHERE url = ?", imageURL)
		}
		return nil, "", false
	}

	return imgData, format, true
}

func handleImageProxy(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" || r.URL.Path == "/favicon.ico" {
		http.NotFound(w, r)
		return
	}

	imageURL := strings.TrimPrefix(r.URL.Path, "/")
	if imageURL == "" {
		http.Error(w, "未指定图片URL", http.StatusBadRequest)
		return
	}

	if !strings.HasPrefix(imageURL, "http://") && !strings.HasPrefix(imageURL, "https://") {
		if strings.HasPrefix(imageURL, "http/") {
			imageURL = strings.Replace(imageURL, "http/", "http:/", 1)
		} else if strings.HasPrefix(imageURL, "https/") {
			imageURL = strings.Replace(imageURL, "https/", "https:/", 1)
		}

		if strings.HasPrefix(imageURL, "http:/") && !strings.HasPrefix(imageURL, "http://") {
			imageURL = strings.Replace(imageURL, "http:/", "http://", 1)
		} else if strings.HasPrefix(imageURL, "https:/") && !strings.HasPrefix(imageURL, "https://") {
			imageURL = strings.Replace(imageURL, "https:/", "https://", 1)
		}
	}

	parsedURL, err := url.Parse(imageURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		http.Error(w, fmt.Sprintf("图片URL无效，必须以 http:// 或 https:// 开头: %v\n提供的URL: %s", err, imageURL), http.StatusBadRequest)
		return
	}

	imgData, format, cacheHit := getFromCache(imageURL)

	if !cacheHit {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(parsedURL.String())
		if err != nil {
			http.Error(w, fmt.Sprintf("图片下载失败: %v", err), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			http.Error(w, fmt.Sprintf("图片下载失败: %s, %s", resp.Status, string(body)), resp.StatusCode)
			return
		}

		rawImgData, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("读取图片数据失败: %v", err), http.StatusInternalServerError)
			return
		}

		img, detectedFormat, err := image.Decode(bytes.NewReader(rawImgData))
		if err != nil {
			http.Error(w, fmt.Sprintf("图片解码失败: %v", err), http.StatusUnsupportedMediaType)
			return
		}

		format = detectedFormat
		var buf bytes.Buffer

		if format == "gif" {
			gifImg, err := gif.DecodeAll(bytes.NewReader(rawImgData))
			if err != nil || len(gifImg.Image) <= 1 {
				format = "webp"
				if err := nativewebp.Encode(&buf, img, nil); err != nil {
					http.Error(w, fmt.Sprintf("WebP 编码失败: %v", err), http.StatusInternalServerError)
					return
				}
			} else {
				format = "gif"
				if err := gif.EncodeAll(&buf, gifImg); err != nil {
					http.Error(w, fmt.Sprintf("GIF 编码失败: %v", err), http.StatusInternalServerError)
					return
				}
			}
		} else {
			format = "webp"
			if err := nativewebp.Encode(&buf, img, nil); err != nil {
				http.Error(w, fmt.Sprintf("WebP 编码失败: %v", err), http.StatusInternalServerError)
				return
			}
		}

		imgData = buf.Bytes()
		originalSize := int64(len(rawImgData))
		compressedSize := int64(len(imgData))
		cachePath := getCacheFilePath(imageURL, format)

		thumbPath := ""
		thumb := generateThumbnail(img, 200, 200)
		if thumb != nil {
			var thumbBuf bytes.Buffer
			if err := nativewebp.Encode(&thumbBuf, thumb, nil); err == nil {
				thumbFileName := strings.TrimSuffix(filepath.Base(cachePath), filepath.Ext(cachePath)) + "_thumb.webp"
				thumbPath = filepath.Join(cacheDir, "thumbs", thumbFileName)
				if err := os.WriteFile(thumbPath, thumbBuf.Bytes(), 0644); err != nil {
					log.Printf("保存缩略图失败: %v", err)
					thumbPath = ""
				}
			} else {
				log.Printf("缩略图编码失败: %v", err)
			}
		}

		if err := os.WriteFile(cachePath, imgData, 0644); err != nil {
			log.Printf("保存缓存失败: %v", err)
		} else {
			updateCacheRecord(imageURL, cachePath, thumbPath, format, false, originalSize, compressedSize)
		}
	} else {
		compressedSize := int64(len(imgData))
		estimatedOriginalSize := compressedSize * 3
		updateCacheRecord(imageURL, "", "", format, true, estimatedOriginalSize, compressedSize)
	}

	switch format {
	case "png":
		w.Header().Set("Content-Type", "image/png")
	case "gif":
		w.Header().Set("Content-Type", "image/gif")
	case "webp":
		w.Header().Set("Content-Type", "image/webp")
	default:
		w.Header().Set("Content-Type", "image/jpeg")
	}

	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(imgData)
	atomic.AddInt64(&requestCount, 1)
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	count := atomic.LoadInt64(&requestCount)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	// 获取缓存统计信息
	dbMutex.Lock()
	var totalHits, totalMisses int
	var cacheFiles int
	var cacheSize int64
	var totalBytesSaved, totalBandwidthSaved int64

	// 获取缓存命中和未命中次数以及节省的空间和流量
	err := db.QueryRow("SELECT total_cache_hits, total_cache_misses, total_bytes_saved, total_bandwidth_saved FROM stats WHERE id = 1").Scan(&totalHits, &totalMisses, &totalBytesSaved, &totalBandwidthSaved)
	if err != nil {
		log.Printf("获取缓存统计失败: %v", err)
		totalHits = 0
		totalMisses = 0
		totalBytesSaved = 0
		totalBandwidthSaved = 0
	}

	// 获取缓存文件数量
	err = db.QueryRow("SELECT COUNT(*) FROM cache").Scan(&cacheFiles)
	if err != nil {
		log.Printf("获取缓存文件数量失败: %v", err)
		cacheFiles = 0
	}

	// 获取缓存大小
	err = filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			cacheSize += info.Size()
		}
		return nil
	})
	if err != nil {
		log.Printf("计算缓存大小失败: %v", err)
	}

	// 计算缓存命中率
	var hitRate float64
	if totalHits+totalMisses > 0 {
		hitRate = float64(totalHits) / float64(totalHits+totalMisses) * 100
	}

	dbMutex.Unlock()

	stats := map[string]interface{}{
		"total_requests":      count,
		"total_cache_hits":    totalHits,
		"total_cache_misses":  totalMisses,
		"cache_hit_rate":      fmt.Sprintf("%.1f%%", hitRate),
		"cache_files":         cacheFiles,
		"cache_size":          cacheSize,
		"cache_size_mb":       float64(cacheSize) / 1024 / 1024,
		"total_bytes_saved":   totalBytesSaved,
		"total_bytes_saved_mb": float64(totalBytesSaved) / 1024 / 1024,
		"total_bandwidth_saved": totalBandwidthSaved,
		"total_bandwidth_saved_mb": float64(totalBandwidthSaved) / 1024 / 1024,
		"current_time":        time.Now().In(localTZ).Format("2006-01-02 15:04:05 MST"),
		"server_timezone":     localTZ.String(),
	}

	json.NewEncoder(w).Encode(stats)
}

func generateThumbnail(img image.Image, maxWidth, maxHeight int) image.Image {
	bounds := img.Bounds()
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()

	scaleX := float64(maxWidth) / float64(origWidth)
	scaleY := float64(maxHeight) / float64(origHeight)
	scale := math.Min(scaleX, scaleY)

	if scale >= 1.0 {
		return img
	}

	newWidth := int(float64(origWidth) * scale)
	newHeight := int(float64(origHeight) * scale)

	thumbnail := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			srcX := int(float64(x) / scale)
			srcY := int(float64(y) / scale)
			if srcX >= origWidth {
				srcX = origWidth - 1
			}
			if srcY >= origHeight {
				srcY = origHeight - 1
			}
			thumbnail.Set(x, y, img.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
		}
	}

	return thumbnail
}

func handleThumbnail(w http.ResponseWriter, r *http.Request) {
	fileName := strings.TrimPrefix(r.URL.Path, "/thumb/")
	if fileName == "" {
		http.Error(w, "缺少文件名", http.StatusBadRequest)
		return
	}

	thumbPath := filepath.Join(cacheDir, "thumbs", fileName)

	if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
		http.Error(w, "缩略图不存在", http.StatusNotFound)
		return
	}

	thumbData, err := os.ReadFile(thumbPath)
	if err != nil {
		http.Error(w, "读取缩略图失败", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(thumbData)
}

type CacheItem struct {
	URL         string    `json:"url"`
	FilePath    string    `json:"file_path"`
	ThumbPath   string    `json:"thumb_path"`
	Format      string    `json:"format"`
	AccessCount int       `json:"access_count"`
	LastAccess  time.Time `json:"last_access"`
	CreatedAt   time.Time `json:"created_at"`
}

type CacheListResponse struct {
	Items      []CacheItem `json:"items"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

func handleCacheList(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	pageSizeStr := r.URL.Query().Get("page_size")
	sortBy := r.URL.Query().Get("sort")
	format := r.URL.Query().Get("format")

	page := 1
	pageSize := 20
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	if r.Header.Get("Accept") != "" && strings.Contains(r.Header.Get("Accept"), "text/html") {
		handleCacheListHTML(w, r, page, pageSize, sortBy)
		return
	}

	var whereClause string
	var args []interface{}
	if format != "" {
		whereClause = "WHERE format = ?"
		args = append(args, format)
	}

	orderBy := "ORDER BY last_access DESC"
	switch sortBy {
	case "access_count":
		orderBy = "ORDER BY access_count DESC"
	case "created_at":
		orderBy = "ORDER BY created_at DESC"
	case "url":
		orderBy = "ORDER BY url ASC"
	}

	dbMutex.Lock()
	defer dbMutex.Unlock()

	var total int
	countQuery := "SELECT COUNT(*) FROM cache"
	if whereClause != "" {
		countQuery += " " + whereClause
	}
	err := db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		log.Printf("查询总数失败: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"查询总数失败"}`))
		return
	}

	offset := (page - 1) * pageSize
	var query string
	if whereClause != "" {
		query = fmt.Sprintf("SELECT url, file_path, thumb_path, format, access_count, last_access, created_at FROM cache %s %s LIMIT ? OFFSET ?", whereClause, orderBy)
	} else {
		query = fmt.Sprintf("SELECT url, file_path, thumb_path, format, access_count, last_access, created_at FROM cache %s LIMIT ? OFFSET ?", orderBy)
	}
	queryArgs := append(args, pageSize, offset)

	rows, err := db.Query(query, queryArgs...)
	if err != nil {
		log.Printf("查询缓存列表失败: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"查询缓存列表失败"}`))
		return
	}
	defer rows.Close()

	var items []CacheItem
	for rows.Next() {
		var item CacheItem
		var lastAccessStr, createdAtStr string
		err := rows.Scan(&item.URL, &item.FilePath, &item.ThumbPath, &item.Format, &item.AccessCount, &lastAccessStr, &createdAtStr)
		if err != nil {
			log.Printf("扫描缓存记录失败: %v", err)
			continue
		}

		if item.LastAccess, err = parseDBTime(lastAccessStr); err != nil {
			log.Printf("解析最后访问时间失败: %v", err)
		}
		if item.CreatedAt, err = parseDBTime(createdAtStr); err != nil {
			log.Printf("解析创建时间失败: %v", err)
		}

		items = append(items, item)
	}

	totalPages := (total + pageSize - 1) / pageSize

	response := CacheListResponse{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("JSON编码失败: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"JSON编码失败"}`))
		return
	}
}

func handleCacheListHTML(w http.ResponseWriter, r *http.Request, page, pageSize int, sortBy string) {
	htmlTemplate := `
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>缓存图片管理</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            margin: 0;
            padding: 20px;
            background-color: #f5f5f5;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background: white;
            border-radius: 8px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
            overflow: hidden;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 20px;
            text-align: center;
        }
        .controls {
            padding: 20px;
            border-bottom: 1px solid #eee;
            display: flex;
            gap: 15px;
            align-items: center;
            flex-wrap: wrap;
        }
        .controls select, .controls input {
            padding: 8px 12px;
            border: 1px solid #ddd;
            border-radius: 4px;
            font-size: 14px;
        }
        .controls button {
            background: #667eea;
            color: white;
            border: none;
            padding: 8px 16px;
            border-radius: 4px;
            cursor: pointer;
            font-size: 14px;
        }
        .controls button:hover {
            background: #5a6fd8;
        }
        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
            gap: 20px;
            padding: 20px;
        }
        .card {
            border: 1px solid #eee;
            border-radius: 8px;
            overflow: hidden;
            transition: transform 0.2s, box-shadow 0.2s;
        }
        .card:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 15px rgba(0,0,0,0.1);
        }
        .card-image {
            width: 100%;
            height: 200px;
            background: #f8f9fa;
            display: flex;
            align-items: center;
            justify-content: center;
            overflow: hidden;
        }
        .card-image img {
            max-width: 100%;
            max-height: 100%;
            object-fit: contain;
        }
        .card-content {
            padding: 15px;
        }
        .card-url {
            font-size: 12px;
            color: #666;
            word-break: break-all;
            margin-bottom: 8px;
            line-height: 1.4;
        }
        .card-meta {
            display: flex;
            justify-content: space-between;
            align-items: center;
            font-size: 12px;
            color: #888;
        }
        .format-badge {
            background: #e3f2fd;
            color: #1976d2;
            padding: 2px 6px;
            border-radius: 3px;
            font-size: 10px;
            font-weight: bold;
            text-transform: uppercase;
        }
        .access-count {
            background: #f3e5f5;
            color: #7b1fa2;
            padding: 2px 6px;
            border-radius: 3px;
            font-size: 10px;
            font-weight: bold;
        }
        .pagination {
            padding: 20px;
            text-align: center;
            border-top: 1px solid #eee;
        }
        .pagination a, .pagination span {
            display: inline-block;
            padding: 8px 12px;
            margin: 0 2px;
            text-decoration: none;
            border: 1px solid #ddd;
            border-radius: 4px;
            color: #333;
        }
        .pagination a:hover {
            background: #f5f5f5;
        }
        .pagination .current {
            background: #667eea;
            color: white;
            border-color: #667eea;
        }
        .stats {
            padding: 15px 20px;
            background: #f8f9fa;
            border-bottom: 1px solid #eee;
            font-size: 14px;
            color: #666;
        }
        .no-data {
            text-align: center;
            padding: 60px 20px;
            color: #999;
        }
        @media (max-width: 768px) {
            .grid {
                grid-template-columns: 1fr;
            }
            .controls {
                flex-direction: column;
                align-items: stretch;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🖼️ 缓存图片管理</h1>
            <p>查看和管理所有缓存的图片文件</p>
        </div>
        
        <div class="controls">
            <select id="sortSelect" onchange="updateList()">
                <option value="last_access">按最后访问时间排序</option>
                <option value="access_count">按访问次数排序</option>
                <option value="created_at">按创建时间排序</option>
                <option value="url">按URL排序</option>
            </select>
            
            <select id="formatSelect" onchange="updateList()">
                <option value="">所有格式</option>
                <option value="webp">WebP</option>
                <option value="gif">GIF</option>
                <option value="png">PNG</option>
                <option value="jpeg">JPEG</option>
            </select>
            
            <input type="number" id="pageSizeInput" placeholder="每页数量" min="1" max="100" value="20" onchange="updateList()">
            
            <button onclick="refreshList()">🔄 刷新</button>
            <button onclick="window.open('/stats', '_blank')">📊 统计信息</button>
        </div>
        
        <div class="stats" id="statsInfo">
            正在加载统计信息...
        </div>
        
        <div class="grid" id="imageGrid">
            正在加载...
        </div>
        
        <div class="pagination" id="pagination">
        </div>
    </div>

    <script>
        let currentPage = {{.Page}};
        let currentPageSize = {{.PageSize}};
        let currentSort = '{{.Sort}}';
        let currentFormat = '';
        
        document.getElementById('sortSelect').value = currentSort;
        document.getElementById('pageSizeInput').value = currentPageSize;
        
        function updateList() {
            currentPage = 1;
            currentSort = document.getElementById('sortSelect').value;
            currentFormat = document.getElementById('formatSelect').value;
            currentPageSize = parseInt(document.getElementById('pageSizeInput').value) || 20;
            loadCacheList();
        }
        
        function refreshList() {
            loadCacheList();
        }
        
        function goToPage(page) {
            currentPage = page;
            loadCacheList();
        }
        
        function loadCacheList() {
            const params = new URLSearchParams({
                page: currentPage,
                page_size: currentPageSize,
                sort: currentSort
            });
            
            if (currentFormat) {
                params.append('format', currentFormat);
            }
            
            fetch('/cache?' + params.toString(), {
                headers: {
                    'Accept': 'application/json'
                }
            })
            .then(response => response.json())
            .then(data => {
                renderImageGrid(data.items);
                renderPagination(data);
                updateStats(data);
            })
            .catch(error => {
                console.error('加载失败:', error);
                document.getElementById('imageGrid').innerHTML = '<div class="no-data">加载失败，请稍后重试</div>';
            });
        }
        
        function renderImageGrid(items) {
            const grid = document.getElementById('imageGrid');
            
            if (!items || items.length === 0) {
                grid.innerHTML = '<div class="no-data">暂无缓存图片</div>';
                return;
            }
            
            grid.innerHTML = items.map(item => {
                const thumbUrl = item.thumb_path ? '/thumb/' + item.thumb_path.split('/').pop() : '';
                const lastAccess = new Date(item.last_access).toLocaleString('zh-CN');
                const createdAt = new Date(item.created_at).toLocaleString('zh-CN');
                
                return '<div class="card">' +
                    '<div class="card-image">' +
                    (thumbUrl ? 
                        '<img src="' + thumbUrl + '" alt="缩略图" onerror="this.style.display=\'none\'; this.nextElementSibling.style.display=\'block\'">' +
                        '<div style="display:none; color:#999; font-size:12px;">无缩略图</div>' :
                        '<div style="color:#999; font-size:12px;">无缩略图</div>'
                    ) +
                    '</div>' +
                    '<div class="card-content">' +
                        '<div class="card-url" title="' + item.url + '">' + item.url + '</div>' +
                        '<div class="card-meta">' +
                            '<div>' +
                                '<span class="format-badge">' + item.format + '</span>' +
                                '<span class="access-count">' + item.access_count + '次访问</span>' +
                            '</div>' +
                        '</div>' +
                        '<div style="font-size:11px; color:#aaa; margin-top:8px;">' +
                            '<div>最后访问: ' + lastAccess + '</div>' +
                            '<div>创建时间: ' + createdAt + '</div>' +
                        '</div>' +
                    '</div>' +
                '</div>';
            }).join('');
        }
        
        function renderPagination(data) {
            const pagination = document.getElementById('pagination');
            
            if (data.total_pages <= 1) {
                pagination.innerHTML = '';
                return;
            }
            
            let html = '';
            
            if (data.page > 1) {
                html += '<a href="#" onclick="goToPage(' + (data.page - 1) + ')">« 上一页</a>';
            }
            
            const startPage = Math.max(1, data.page - 2);
            const endPage = Math.min(data.total_pages, data.page + 2);
            
            if (startPage > 1) {
                html += '<a href="#" onclick="goToPage(1)">1</a>';
                if (startPage > 2) {
                    html += '<span>...</span>';
                }
            }
            
            for (let i = startPage; i <= endPage; i++) {
                if (i === data.page) {
                    html += '<span class="current">' + i + '</span>';
                } else {
                    html += '<a href="#" onclick="goToPage(' + i + ')">' + i + '</a>';
                }
            }
            
            if (endPage < data.total_pages) {
                if (endPage < data.total_pages - 1) {
                    html += '<span>...</span>';
                }
                html += '<a href="#" onclick="goToPage(' + data.total_pages + ')">' + data.total_pages + '</a>';
            }
            
            if (data.page < data.total_pages) {
                html += '<a href="#" onclick="goToPage(' + (data.page + 1) + ')">下一页 »</a>';
            }
            
            pagination.innerHTML = html;
        }
        
        function updateStats(data) {
            const statsInfo = document.getElementById('statsInfo');
            statsInfo.innerHTML = 
                '📊 共 <strong>' + data.total + '</strong> 个缓存文件 | ' +
                '📄 当前第 <strong>' + data.page + '</strong> 页，共 <strong>' + data.total_pages + '</strong> 页 | ' +
                '📦 每页显示 <strong>' + data.page_size + '</strong> 个';
        }
        
        document.addEventListener('DOMContentLoaded', function() {
            loadCacheList();
        });
    </script>
</body>
</html>
`

	tmpl, err := template.New("cache").Parse(htmlTemplate)
	if err != nil {
		http.Error(w, "模板解析失败", http.StatusInternalServerError)
		return
	}

	data := struct {
		Page     int
		PageSize int
		Sort     string
	}{
		Page:     page,
		PageSize: pageSize,
		Sort:     sortBy,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, data)
}

func formatTimeForDB(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

func parseDBTime(timeStr string) (time.Time, error) {
	t, err := time.Parse("2006-01-02 15:04:05", timeStr)
	if err != nil {
		return time.Time{}, err
	}
	
	return t.UTC().In(localTZ), nil
}
