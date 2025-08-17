package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"math"
	"math/rand"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/HugoSmits86/nativewebp"
	_ "modernc.org/sqlite"
)

// CacheEntry 内存缓存条目
type CacheEntry struct {
	URL         string
	FilePath    string
	ThumbPath   string
	Format      string
	AccessCount int64
	LastAccess  time.Time
	CreatedAt   time.Time
	Dirty       bool // 标记是否需要写入数据库
	Size        int64 // 缓存文件大小
	prev        *CacheEntry // LRU链表前向指针
	next        *CacheEntry // LRU链表后向指针
}

// LRUCache LRU缓存管理器
type LRUCache struct {
	mu          sync.RWMutex
	entries     map[string]*CacheEntry
	head        *CacheEntry // 最近使用的
	tail        *CacheEntry // 最久未使用的
	maxEntries  int
	maxSizeMB   int
	currentSize int64
}

// CacheConfig 缓存配置
type CacheConfig struct {
	MaxMemCacheEntries int           `json:"max_mem_cache_entries"` // 最大内存缓存条目数
	MaxMemCacheSizeMB  int           `json:"max_mem_cache_size_mb"` // 最大内存缓存大小(MB)
	MaxDiskCacheSizeMB int           `json:"max_disk_cache_size_mb"` // 最大磁盘缓存大小(MB)
	CleanupIntervalMin int           `json:"cleanup_interval_min"`   // 清理间隔(分钟)
	AccessWindowMin    int           `json:"access_window_min"`      // 访问时间窗口(分钟)
	SyncIntervalSec    int           `json:"sync_interval_sec"`      // 数据库同步间隔(秒)
	CacheValidityMin   int           `json:"cache_validity_min"`     // 缓存有效期(分钟)
}

// Language 语言包
type Language struct {
	Code string
	Name string
	UI   map[string]string
}

// 定义语言包
var languages = map[string]*Language{
	"zh": {
		Code: "zh",
		Name: "中文",
		UI: map[string]string{
			// 页面标题
			"title": "缓存管理",
			"stats_title": "实时统计",
			"config_title": "缓存配置",
			
			// 按钮
			"btn_refresh": "刷新",
			"btn_stats": "统计信息",
			"btn_toggle_cache": "切换缓存",
			"btn_sync": "立即同步",
			"btn_config": "配置",
			"btn_refresh_stats": "刷新统计",
			"btn_save": "保存配置",
			"btn_cancel": "取消",
			"btn_delete": "删除",
			"btn_login": "登录",
			"btn_logout": "退出",
			
			// 标签
			"label_memory_cache": "内存缓存",
			"label_enabled": "启用",
			"label_disabled": "禁用",
			"label_page_size": "每页显示",
			"label_sort": "排序",
			"label_filter": "筛选格式",
			"label_all": "全部",
			"label_password": "密码",
			
			// 统计信息
			"stat_total_requests": "总请求数",
			"stat_cache_hits": "缓存命中",
			"stat_cache_misses": "缓存未命中",
			"stat_hit_rate": "命中率",
			"stat_cache_files": "缓存文件",
			"stat_cache_size": "缓存大小",
			"stat_space_saved": "节省空间",
			"stat_bandwidth_saved": "节省带宽",
			
			// 配置项
			"config_max_mem_entries": "内存缓存最大条目数",
			"config_max_mem_size": "内存缓存最大大小 (MB)",
			"config_max_disk_size": "磁盘缓存最大大小 (MB)",
			"config_cleanup_interval": "清理间隔 (分钟)",
			"config_access_window": "访问时间窗口 (分钟)",
			"config_sync_interval": "数据库同步间隔 (秒)",
			"config_cache_validity": "缓存有效期 (分钟)",
			"config_access_window_hint": "超过此时间未访问的条目优先清理",
			
			// 表格头
			"table_preview": "预览",
			"table_url": "原始URL",
			"table_size": "大小",
			"table_format": "格式",
			"table_access_count": "访问次数",
			"table_last_access": "最后访问",
			"table_created": "创建时间",
			"table_actions": "操作",
			
			// 消息
			"msg_loading": "正在加载...",
			"msg_config_updated": "配置已更新！部分设置将在下次启动时完全生效。",
			"msg_config_save_failed": "保存配置失败",
			"msg_cache_toggled": "内存缓存已",
			"msg_synced": "已同步到数据库",
			"msg_deleted": "已删除",
			"msg_login_failed": "密码错误，请重试",
			"msg_no_data": "暂无数据",
			
			// 首页翻译
			"service_title": "图片代理服务",
			"usage_title": "使用方法：",
			"query_param_method": "查询参数方式（推荐，保留双斜杠）：",
			"encoded_path_method": "编码路径方式（用 _DS_ 代表 //）：",
			"standard_path_method": "标准路径方式：",
			"format_conversion_title": "格式转换：",
			"force_webp_conversion": "强制转换为 WebP（默认行为）：",
			"keep_original_format": "保持原始格式：",
			"image_resize_title": "图片尺寸调整：",
			"specify_width": "指定宽度（高度自动按比例）：",
			"specify_height": "指定高度（宽度自动按比例）：",
			"specify_both_dimensions": "指定宽度和高度（保持纵横比，适应框内）：",
			"combined_params": "组合参数（缩放 + 格式 + 质量）：",
			"resize_mode_title": "缩放模式（mode 参数）：",
			"mode_fit_default": "（默认）- 适应框内，保持纵横比：",
			"mode_fit_desc": "图片完全显示在指定尺寸内，可能有空白区域",
			"mode_fill": "填充整个框，裁剪多余部分：",
			"mode_fill_desc": "图片填满整个框，可能裁剪掉部分内容",
			"mode_stretch": "拉伸到精确尺寸：",
			"mode_stretch_desc": "强制拉伸到指定尺寸，可能导致图片变形",
			"mode_pad": "适应框内并添加白色边距：",
			"mode_pad_desc": "保持纵横比，用白色填充空白区域",
			"management_pages_title": "管理页面：",
			"cache_management": "缓存管理",
			"statistics_json": "统计信息（JSON）",
			"image_upload": "图片上传",
			"backend_note": "长期存储后端基于",
		},
	},
	"en": {
		Code: "en",
		Name: "English",
		UI: map[string]string{
			// Page titles
			"title": "Cache Management",
			"stats_title": "Live Statistics",
			"config_title": "Cache Configuration",
			
			// Buttons
			"btn_refresh": "Refresh",
			"btn_stats": "Statistics",
			"btn_toggle_cache": "Toggle Cache",
			"btn_sync": "Sync Now",
			"btn_config": "Config",
			"btn_refresh_stats": "Refresh Stats",
			"btn_save": "Save Config",
			"btn_cancel": "Cancel",
			"btn_delete": "Delete",
			"btn_login": "Login",
			"btn_logout": "Logout",
			
			// Labels
			"label_memory_cache": "Memory Cache",
			"label_enabled": "Enabled",
			"label_disabled": "Disabled",
			"label_page_size": "Per Page",
			"label_sort": "Sort",
			"label_filter": "Filter Format",
			"label_all": "All",
			"label_password": "Password",
			
			// Statistics
			"stat_total_requests": "Total Requests",
			"stat_cache_hits": "Cache Hits",
			"stat_cache_misses": "Cache Misses",
			"stat_hit_rate": "Hit Rate",
			"stat_cache_files": "Cache Files",
			"stat_cache_size": "Cache Size",
			"stat_space_saved": "Space Saved",
			"stat_bandwidth_saved": "Bandwidth Saved",
			
			// Configuration
			"config_max_mem_entries": "Max Memory Cache Entries",
			"config_max_mem_size": "Max Memory Cache Size (MB)",
			"config_max_disk_size": "Max Disk Cache Size (MB)",
			"config_cleanup_interval": "Cleanup Interval (min)",
			"config_access_window": "Access Time Window (min)",
			"config_sync_interval": "DB Sync Interval (sec)",
			"config_cache_validity": "Cache Validity (min)",
			"config_access_window_hint": "Entries not accessed within this time will be cleaned first",
			
			// Table headers
			"table_preview": "Preview",
			"table_url": "Original URL",
			"table_size": "Size",
			"table_format": "Format",
			"table_access_count": "Access Count",
			"table_last_access": "Last Access",
			"table_created": "Created",
			"table_actions": "Actions",
			
			// Messages
			"msg_loading": "Loading...",
			"msg_config_updated": "Configuration updated! Some settings will take full effect on next restart.",
			"msg_config_save_failed": "Failed to save configuration",
			"msg_cache_toggled": "Memory cache has been ",
			"msg_synced": "Synced to database",
			"msg_deleted": "Deleted",
			"msg_login_failed": "Wrong password, please try again",
			"msg_no_data": "No data",
			
			// Homepage translations
			"service_title": "Image Proxy Service",
			"usage_title": "Usage:",
			"query_param_method": "Query parameter method (recommended, preserves double slashes):",
			"encoded_path_method": "Encoded path method (use _DS_ for //):",
			"standard_path_method": "Standard path method:",
			"format_conversion_title": "Format Conversion:",
			"force_webp_conversion": "Force WebP conversion (default behavior):",
			"keep_original_format": "Keep original format:",
			"image_resize_title": "Image Resizing:",
			"specify_width": "Specify width (height auto-scales):",
			"specify_height": "Specify height (width auto-scales):",
			"specify_both_dimensions": "Specify both width and height (maintains aspect ratio):",
			"combined_params": "Combined parameters (resize + format + quality):",
			"resize_mode_title": "Resize Modes (mode parameter):",
			"mode_fit_default": "(default) - Fit within bounds, maintain aspect ratio:",
			"mode_fit_desc": "Image fully displayed within specified dimensions, may have blank areas",
			"mode_fill": "Fill entire frame, crop excess:",
			"mode_fill_desc": "Image fills entire frame, may crop some content",
			"mode_stretch": "Stretch to exact dimensions:",
			"mode_stretch_desc": "Force stretch to specified dimensions, may distort image",
			"mode_pad": "Fit within bounds with white padding:",
			"mode_pad_desc": "Maintain aspect ratio, fill blank areas with white",
			"management_pages_title": "Management Pages:",
			"cache_management": "Cache Management",
			"statistics_json": "Statistics (JSON)",
			"image_upload": "Image Upload",
			"backend_note": "Long-term storage backend based on",
		},
	},
}

// StorageBackend 存储后端接口
type StorageBackend interface {
	// Store 存储文件，返回文件ID
	Store(data []byte, metadata map[string]string) (string, error)
	// Get 获取文件
	Get(id string) ([]byte, error)
	// Exists 检查文件是否存在
	Exists(id string) bool
	// Delete 删除文件
	Delete(id string) error
	// Name 返回存储后端名称
	Name() string
}

// StorageManager 存储管理器，管理多层存储
type StorageManager struct {
	backends []StorageBackend
	mu       sync.RWMutex
}

// MemoryStorage 内存存储后端
type MemoryStorage struct {
	cache    *LRUCache
	data     map[string][]byte
	mu       sync.RWMutex
	maxSize  int64
	currSize int64
}

// LocalStorage 本地文件存储后端
type LocalStorage struct {
	basePath string
	mu       sync.RWMutex
}

// IOBackendStorage 远程io存储后端
type IOBackendStorage struct {
	apiURL   string
	apiKey   string
	client   *http.Client
	enabled  bool
}

// StorageConfig 存储配置
type StorageConfig struct {
	EnableMemory   bool   `json:"enable_memory"`
	EnableLocal    bool   `json:"enable_local"`
	EnableRemote   bool   `json:"enable_remote"`
	MemoryMaxSize  int64  `json:"memory_max_size"`
	LocalPath      string `json:"local_path"`
	RemoteURL      string `json:"remote_url"`
	RemoteAPIKey   string `json:"remote_api_key"`
}

var (
	requestCount int64
	cacheDir     = "cache"
	db           *sql.DB
	dbMutex      sync.Mutex
	cacheHits    int64
	cacheMisses  int64
	cacheMutex   sync.RWMutex
	maxCacheSize = int64(100 * 1024 * 1024) // 100MB
	logFile      *os.File
	logMutex     sync.Mutex
	logSize      int64
	maxLogSize   = int64(10 * 1024 * 1024) // 10MB per log file
	httpServer   *http.Server              // HTTP服务器引用，用于优雅关闭
	ioBackendURL = "http://localhost:7777" // io 后端服务地址
	ioAPIKey     = "" // io 后端API密钥
	ioProcess    *exec.Cmd // io 后端进程
	shutdownChan = make(chan struct{})     // 关闭信号通道
	
	// 全局存储管理器
	storageManager *StorageManager
	
	// 默认存储配置
	defaultStorageConfig = StorageConfig{
		EnableMemory:  true,
		EnableLocal:   true,
		EnableRemote:  false,
		MemoryMaxSize: 50 * 1024 * 1024, // 50MB内存缓存
		LocalPath:     "uploads",
		RemoteURL:     "http://localhost:7777",
		RemoteAPIKey:  "",
	}
	
	// 内存缓存相关
	lruCache      *LRUCache  // LRU缓存管理器
	useMemCache   bool = true // 默认启用内存缓存
	lastDBSync      time.Time    // 上次数据库同步时间
	adminPassword   string       // 管理员密码
	
	// 内存缓存池配置
	cacheConfig = &CacheConfig{
		MaxMemCacheEntries: 1000,
		MaxMemCacheSizeMB:  50,
		MaxDiskCacheSizeMB: 100,
		CleanupIntervalMin: 5,
		AccessWindowMin:    30,
		SyncIntervalSec:    30,
		CacheValidityMin:   10,
	}
	cleanupStopChan    = make(chan bool)   // 用于停止清理协程的通道
	syncStopChan       = make(chan bool)   // 用于停止同步协程的通道
	currentLang        = "zh"               // 默认语言
	startTime          = time.Now()         // 服务启动时间
	
	// 缓冲池，用于复用内存
	bufferPool = sync.Pool{
		New: func() interface{} {
			// 创建 32KB 的缓冲区
			return make([]byte, 32*1024)
		},
	}
	
	// 大缓冲池，用于图片数据
	largeBufferPool = sync.Pool{
		New: func() interface{} {
			// 创建 1MB 的缓冲区用于图片
			return &bytes.Buffer{}
		},
	}
)

// getLang 根据请求获取语言设置
func getLang(r *http.Request) *Language {
	// 优先从cookie获取
	if cookie, err := r.Cookie("lang"); err == nil {
		if lang, ok := languages[cookie.Value]; ok {
			return lang
		}
	}
	
	// 从Accept-Language头获取
	acceptLang := r.Header.Get("Accept-Language")
	if strings.Contains(acceptLang, "zh") {
		return languages["zh"]
	} else if strings.Contains(acceptLang, "en") {
		return languages["en"]
	}
	
	// 返回默认语言
	return languages[currentLang]
}

// downloadAndStartIOBackend 下载并启动 io 存储后端（可选）
func downloadAndStartIOBackend(config *StorageConfig) error {
	if !config.EnableRemote {
		log.Println("远程io存储未启用")
		return nil
	}
	log.Println("正在检查 io 存储后端...")
	
	// 创建 io-backend 目录
	backendDir := "io-backend"
	if err := os.MkdirAll(backendDir, 0755); err != nil {
		return fmt.Errorf("创建后端目录失败: %v", err)
	}
	
	// 检测系统架构
	var platform string
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			platform = "darwin-arm64"
		} else {
			platform = "darwin-amd64"
		}
	case "linux":
		if runtime.GOARCH == "arm64" {
			platform = "linux-arm64"
		} else {
			platform = "linux-amd64"
		}
	case "windows":
		platform = "windows-amd64"
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
	
	binaryName := "io"
	if runtime.GOOS == "windows" {
		binaryName = "io.exe"
	}
	binaryPath := filepath.Join(backendDir, binaryName)
	
	// 检查二进制文件是否已存在
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		log.Printf("正在下载 io 存储后端 (%s)...", platform)
		
		// 下载最新版本
		downloadURL := fmt.Sprintf("https://github.com/zots0127/io/releases/latest/download/io-%s.tar.gz", platform)
		
		resp, err := http.Get(downloadURL)
		if err != nil {
			return fmt.Errorf("下载失败: %v", err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
		}
		
		// 解压 tar.gz
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("解压失败: %v", err)
		}
		defer gzReader.Close()
		
		tarReader := tar.NewReader(gzReader)
		
		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("读取tar失败: %v", err)
			}
			
			// 只提取 io 二进制文件
			if header.Name == binaryName || header.Name == "./"+binaryName {
				outFile, err := os.OpenFile(binaryPath, os.O_CREATE|os.O_WRONLY, 0755)
				if err != nil {
					return fmt.Errorf("创建文件失败: %v", err)
				}
				
				if _, err := io.Copy(outFile, tarReader); err != nil {
					outFile.Close()
					return fmt.Errorf("写入文件失败: %v", err)
				}
				outFile.Close()
				
				log.Println("io 存储后端下载完成")
				break
			}
		}
	}
	
	// 生成随机 API 密钥
	if config.RemoteAPIKey == "" {
		rand.Seed(time.Now().UnixNano())
		b := make([]byte, 32)
		for i := range b {
			b[i] = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"[rand.Intn(62)]
		}
		config.RemoteAPIKey = string(b)
		log.Printf("生成 io API 密钥: %s", config.RemoteAPIKey)
	}
	ioAPIKey = config.RemoteAPIKey
	
	// 创建 io 存储目录
	ioStorageDir := filepath.Join(backendDir, "storage")
	if err := os.MkdirAll(ioStorageDir, 0755); err != nil {
		return fmt.Errorf("创建存储目录失败: %v", err)
	}
	
	// 启动 io 后端
	log.Println("正在启动 io 存储后端...")
	ioProcess = exec.Command(binaryPath,
		"-port", "7777",
		"-storage", ioStorageDir,
		"-db", filepath.Join(backendDir, "io.db"),
		"-api-key", ioAPIKey,
	)
	
	ioProcess.Stdout = os.Stdout
	ioProcess.Stderr = os.Stderr
	
	if err := ioProcess.Start(); err != nil {
		return fmt.Errorf("启动 io 后端失败: %v", err)
	}
	
	// 等待后端启动
	time.Sleep(2 * time.Second)
	
	// 检查后端是否正常运行
	resp, err := http.Get(ioBackendURL + "/health")
	if err == nil {
		resp.Body.Close()
		log.Println("io 存储后端启动成功")
	} else {
		log.Printf("警告: io 后端健康检查失败: %v", err)
	}
	
	return nil
}

func main() {
	log.Println("正在初始化服务...")
	
	// 加载存储配置（可以从环境变量或配置文件读取）
	storageConfig := defaultStorageConfig
	
	// 从环境变量读取配置
	if os.Getenv("STORAGE_MEMORY") == "false" {
		storageConfig.EnableMemory = false
	}
	if os.Getenv("STORAGE_LOCAL") == "false" {
		storageConfig.EnableLocal = false
	}
	if os.Getenv("STORAGE_REMOTE") == "true" {
		storageConfig.EnableRemote = true
	}
	if remoteURL := os.Getenv("STORAGE_REMOTE_URL"); remoteURL != "" {
		storageConfig.RemoteURL = remoteURL
	}
	if apiKey := os.Getenv("STORAGE_REMOTE_APIKEY"); apiKey != "" {
		storageConfig.RemoteAPIKey = apiKey
	}
	
	// 如果启用远程存储，尝试启动 io 后端
	if storageConfig.EnableRemote {
		if err := downloadAndStartIOBackend(&storageConfig); err != nil {
			log.Printf("警告: 无法启动 io 存储后端: %v", err)
			storageConfig.EnableRemote = false
		}
	}
	
	// 初始化存储管理器
	storageManager = NewStorageManager(storageConfig)
	log.Printf("存储配置: 内存=%v, 本地=%v, 远程=%v", 
		storageConfig.EnableMemory, 
		storageConfig.EnableLocal, 
		storageConfig.EnableRemote)
	
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Fatalf("创建缓存目录失败: %v", err)
	}

	thumbDir := filepath.Join(cacheDir, "thumbs")
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		log.Fatalf("创建缩略图目录失败: %v", err)
	}
	
	// 创建上传目录
	uploadsDir := "uploads"
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		log.Fatalf("创建上传目录失败: %v", err)
	}

	// 初始化日志系统
	initLogger()
	defer closeLogger()
	
	// 加载管理员密码
	loadAdminPassword()
	
	// 加载缓存配置
	loadCacheConfig()
	
	// 初始化LRU缓存
	lruCache = NewLRUCache(cacheConfig.MaxMemCacheEntries, cacheConfig.MaxMemCacheSizeMB)

	initDB()
	
	// 从数据库加载到内存缓存
	if useMemCache {
		loadCacheFromDB()
		// 启动定时同步
		go syncMemCacheToDB()
		// 启动内存缓存清理
		go cleanupMemCache()
	}
	
	// 优雅关闭处理
	setupGracefulShutdown()

	go cleanExpiredCache()

	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/stats", handleStats)
	http.HandleFunc("/upload", handleUpload)
	http.HandleFunc("/api/upload", handleAPIUpload)
	http.HandleFunc("/storage/", handleStorageFiles)
	http.HandleFunc("/uploads/", handleUploads)  // 保留兼容旧的本地上传
	http.HandleFunc("/io/", handleIOFiles)       // 保留兼容旧的io后端
	http.HandleFunc("/cache/control", handleCacheControl)
	http.HandleFunc("/cache", handleCacheList)
	http.HandleFunc("/thumb/", handleThumbnail)
	http.HandleFunc("/", handleImageProxy)

	
	// 自动查找可用端口
	port := 8080
	maxPort := 8100 // 最多尝试到8100端口
	var listener net.Listener
	var err error
	
	for port <= maxPort {
		addr := fmt.Sprintf(":%d", port)
		listener, err = net.Listen("tcp", addr)
		if err == nil {
			// 端口可用
			fmt.Printf("Server started on http://0.0.0.0:%d\n", port)
			fmt.Printf("Cache management: http://0.0.0.0:%d/cache\n", port)
			break
		}
		// 端口被占用，尝试下一个
		log.Printf("Port %d is busy, trying %d...\n", port, port+1)
		port++
	}
	
	if listener == nil {
		log.Fatalf("No available port found between 8080 and %d", maxPort)
	}
	
	// 创建 HTTP 服务器
	httpServer = &http.Server{
		Handler:      http.DefaultServeMux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	
	// 使用找到的可用监听器启动服务
	if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server error: %v", err)
	}
}

// logWriter 自定义日志写入器，用于跟踪日志大小
type logWriter struct {
	file *os.File
	size *int64
	mu   *sync.Mutex
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	n, err = w.file.Write(p)
	if err == nil {
		atomic.AddInt64(w.size, int64(n))
	}
	return n, err
}

// initLogger 初始化日志系统，支持日志文件轮转
func initLogger() {
	// 创建日志目录
	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Printf("创建日志目录失败: %v\n", err)
		return
	}

	// 生成日志文件名
	logFileName := filepath.Join(logDir, fmt.Sprintf("imgproxy_%s.log", time.Now().Format("2006-01-02")))
	
	// 打开或创建日志文件
	var err error
	logFile, err = os.OpenFile(logFileName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("打开日志文件失败: %v\n", err)
		return
	}

	// 获取文件大小
	if info, err := logFile.Stat(); err == nil {
		logSize = info.Size()
	}

	// 创建自定义日志写入器
	lw := &logWriter{
		file: logFile,
		size: &logSize,
		mu:   &logMutex,
	}

	// 设置日志输出到文件和控制台
	multiWriter := io.MultiWriter(os.Stdout, lw)
	log.SetOutput(multiWriter)
	log.SetFlags(log.Ldate | log.Ltime)
	
	// 启动日志轮转检查
	go logRotationCheck()
}

// loadAdminPassword 从.pass文件加载管理员密码
func loadAdminPassword() {
	data, err := os.ReadFile(".pass")
	if err != nil {
		// 如果文件不存在，生成随机密码
		adminPassword = generateRandomPassword()
		// 将生成的密码写入.pass文件
		if err := os.WriteFile(".pass", []byte(adminPassword), 0600); err != nil {
			log.Printf("写入密码文件失败: %v", err)
		} else {
			log.Printf("已生成随机密码并保存到.pass文件: %s", adminPassword)
		}
		return
	}
	adminPassword = strings.TrimSpace(string(data))
	log.Println("已加载管理员密码")
}

// generateRandomPassword 生成8位随机密码（数字+字母）
func generateRandomPassword() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rand.Seed(time.Now().UnixNano())
	password := make([]byte, 8)
	for i := range password {
		password[i] = charset[rand.Intn(len(charset))]
	}
	return string(password)
}

// loadCacheConfig 从config.json文件加载缓存配置
func loadCacheConfig() {
	data, err := os.ReadFile("config.json")
	if err != nil {
		// 文件不存在，使用默认配置并保存
		saveCacheConfig()
		log.Println("使用默认缓存配置")
		return
	}
	
	var config CacheConfig
	if err := json.Unmarshal(data, &config); err != nil {
		log.Printf("解析配置文件失败: %v，使用默认配置", err)
		return
	}
	
	// 验证配置值的合理性
	if config.MaxMemCacheEntries <= 0 {
		config.MaxMemCacheEntries = 1000
	}
	if config.MaxMemCacheSizeMB <= 0 {
		config.MaxMemCacheSizeMB = 50
	}
	if config.MaxDiskCacheSizeMB <= 0 {
		config.MaxDiskCacheSizeMB = 100
	}
	if config.CleanupIntervalMin <= 0 {
		config.CleanupIntervalMin = 5
	}
	if config.AccessWindowMin <= 0 {
		config.AccessWindowMin = 30
	}
	if config.SyncIntervalSec <= 0 {
		config.SyncIntervalSec = 30
	}
	if config.CacheValidityMin <= 0 {
		config.CacheValidityMin = 10
	}
	
	cacheConfig = &config
	log.Printf("已加载缓存配置: %+v", cacheConfig)
}

// saveCacheConfig 保存缓存配置到config.json文件
func saveCacheConfig() error {
	data, err := json.MarshalIndent(cacheConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}
	
	if err := os.WriteFile("config.json", data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}
	
	log.Println("已保存缓存配置到config.json")
	return nil
}

// loadCacheFromDB 从数据库加载缓存到内存
func loadCacheFromDB() {
	log.Println("正在从数据库加载缓存到内存...")
	
	dbMutex.Lock()
	defer dbMutex.Unlock()
	
	rows, err := db.Query("SELECT url, file_path, thumb_path, format, access_count, last_access, created_at FROM cache")
	if err != nil {
		log.Printf("加载缓存失败: %v", err)
		return
	}
	defer rows.Close()
	
	count := 0
	for rows.Next() {
		var entry CacheEntry
		var lastAccessStr, createdAtStr string
		
		err := rows.Scan(&entry.URL, &entry.FilePath, &entry.ThumbPath, 
			&entry.Format, &entry.AccessCount, &lastAccessStr, &createdAtStr)
		if err != nil {
			log.Printf("读取缓存记录失败: %v", err)
			continue
		}
		
		// 解析时间
		for _, format := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05"} {
			if entry.LastAccess, err = time.Parse(format, lastAccessStr); err == nil {
				break
			}
		}
		for _, format := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05"} {
			if entry.CreatedAt, err = time.Parse(format, createdAtStr); err == nil {
				break
			}
		}
		
		entry.Dirty = false
		entry.Size = 0 // 稍后统计实际大小
		if fileInfo, err := os.Stat(entry.FilePath); err == nil {
			entry.Size = fileInfo.Size()
		}
		lruCache.Put(entry.URL, &entry)
		count++
	}
	
	log.Printf("已加载 %d 条缓存记录到内存", count)
}

// syncMemCacheToDB 定期同步内存缓存到数据库
func syncMemCacheToDB() {
	ticker := time.NewTicker(time.Duration(cacheConfig.SyncIntervalSec) * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			syncToDB()
		case <-syncStopChan:
			log.Println("停止数据库同步")
			return
		}
	}
}

// syncToDB 执行实际的同步操作
func syncToDB() {
	if !useMemCache {
		return
	}
	
	// 使用LRU缓存的方法收集需要同步的条目
	var toSync []*CacheEntry
	for _, entry := range lruCache.GetAll() {
		if entry.Dirty {
			entryCopy := *entry
			toSync = append(toSync, &entryCopy)
		}
	}
	
	if len(toSync) == 0 {
		return
	}
	
	log.Printf("开始同步 %d 条记录到数据库", len(toSync))
	
	dbMutex.Lock()
	defer dbMutex.Unlock()
	
	// 开始事务
	tx, err := db.Begin()
	if err != nil {
		log.Printf("开始事务失败: %v", err)
		return
	}
	
	for _, entry := range toSync {
		_, err := tx.Exec(`
			INSERT OR REPLACE INTO cache 
			(url, file_path, thumb_path, format, access_count, last_access, created_at) 
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			entry.URL, entry.FilePath, entry.ThumbPath, entry.Format,
			entry.AccessCount, entry.LastAccess.Format(time.RFC3339),
			entry.CreatedAt.Format(time.RFC3339))
		
		if err != nil {
			log.Printf("同步缓存记录失败: %v", err)
			tx.Rollback()
			return
		}
	}
	
	if err := tx.Commit(); err != nil {
		log.Printf("提交事务失败: %v", err)
		return
	}
	
	// 标记已同步
	for _, entry := range toSync {
		if cached, exists := lruCache.Get(entry.URL); exists {
			cached.Dirty = false
		}
	}
	
	lastDBSync = time.Now()
	log.Printf("成功同步 %d 条记录到数据库", len(toSync))
}

// cleanupMemCache 定期清理过期的缓存
func cleanupMemCache() {
	ticker := time.NewTicker(time.Duration(cacheConfig.CleanupIntervalMin) * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			if !useMemCache {
				continue
			}
			
			// LRU缓存自动处理大小限制，这里只需要清理过期的条目
			now := time.Now()
			cacheValidity := time.Duration(cacheConfig.CacheValidityMin) * time.Minute
			
			expiredCount := 0
			for key, entry := range lruCache.GetAll() {
				if now.Sub(entry.LastAccess) > cacheValidity {
					// 同步脏数据
					if entry.Dirty {
						syncSingleEntry(key, entry)
					}
					// 从LRU缓存中删除（会自动删除文件）
					lruCache.Remove(key)
					expiredCount++
				}
			}
			
			if expiredCount > 0 {
				log.Printf("清理了 %d 个过期缓存条目", expiredCount)
			}
			
			// 显示缓存状态
			log.Printf("LRU缓存状态: %d 条目, 约 %.2f MB", 
				lruCache.Len(), 
				float64(lruCache.currentSize)/(1024*1024))
			
		case <-cleanupStopChan:
			log.Println("停止缓存清理")
			return
		}
	}
}

// syncSingleEntry 同步单个缓存条目到数据库
func syncSingleEntry(url string, entry *CacheEntry) {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	
	// 检查是否存在
	var exists bool
	err := db.QueryRow("SELECT 1 FROM cache WHERE url = ?", url).Scan(&exists)
	
	if err == sql.ErrNoRows {
		// 插入新记录
		_, err = db.Exec(
			`INSERT INTO cache (url, file_path, thumb_path, format, access_count, last_access, created_at) 
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			url, entry.FilePath, entry.ThumbPath, entry.Format, 
			entry.AccessCount, entry.LastAccess, entry.CreatedAt,
		)
	} else if err == nil {
		// 更新现有记录
		_, err = db.Exec(
			`UPDATE cache SET access_count = ?, last_access = ? WHERE url = ?`,
			entry.AccessCount, entry.LastAccess, url,
		)
	}
	
	if err != nil {
		log.Printf("同步单个缓存条目失败: %v", err)
	}
}


// closeLogger 关闭日志文件
func closeLogger() {
	logMutex.Lock()
	defer logMutex.Unlock()
	
	if logFile != nil {
		logFile.Close()
	}
}

// logRotationCheck 定期检查并轮转日志
func logRotationCheck() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		// 使用原子操作读取日志大小
		currentSize := atomic.LoadInt64(&logSize)
		
		// 检查日志文件大小
		if currentSize >= maxLogSize {
			logMutex.Lock()
			// 关闭当前日志文件
			if logFile != nil {
				logFile.Close()
			}
			
			// 创建新的日志文件
			logDir := "logs"
			timestamp := time.Now().Format("2006-01-02_15-04-05")
			newLogFileName := filepath.Join(logDir, fmt.Sprintf("imgproxy_%s.log", timestamp))
			
			var err error
			logFile, err = os.OpenFile(newLogFileName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				fmt.Printf("创建新日志文件失败: %v\n", err)
				logMutex.Unlock()
				continue
			}
			
			// 重置日志大小
			atomic.StoreInt64(&logSize, 0)
			
			// 创建新的日志写入器
			lw := &logWriter{
				file: logFile,
				size: &logSize,
				mu:   &logMutex,
			}
			
			// 更新日志输出
			multiWriter := io.MultiWriter(os.Stdout, lw)
			log.SetOutput(multiWriter)
			
			log.Println("日志文件已轮转")
			logMutex.Unlock()
		}
		
		// 清理旧日志文件（保留最近7天的日志）
		cleanOldLogs()
	}
}

// cleanOldLogs 清理旧的日志文件
func cleanOldLogs() {
	logDir := "logs"
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}

	cutoffTime := time.Now().AddDate(0, 0, -7) // 7天前
	
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		
		info, err := entry.Info()
		if err != nil {
			continue
		}
		
		// 如果文件修改时间早于7天前，删除它
		if info.ModTime().Before(cutoffTime) {
			filePath := filepath.Join(logDir, entry.Name())
			if err := os.Remove(filePath); err != nil {
				log.Printf("删除旧日志文件失败 %s: %v", filePath, err)
			} else {
				log.Printf("已删除旧日志文件: %s", filePath)
			}
		}
	}
}

func initDB() {
	var err error
	// 修改驱动名称从sqlite3为sqlite
	db, err = sql.Open("sqlite", "./imgproxy.db")
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}

	// 设置数据库参数，支持错误恢复
	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA temp_store = MEMORY;",
		"PRAGMA busy_timeout = 10000;",  // 增加超时时间到10秒
		"PRAGMA cache_size = -64000;",    // 64MB缓存
		"PRAGMA mmap_size = 268435456;",  // 256MB内存映射
	}
	
	for _, pragma := range pragmas {
		if _, err = db.Exec(pragma); err != nil {
			log.Printf("Setting database parameter failed [%s]: %v", pragma, err)
		}
	}
	
	// 设置连接池参数
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 	Create cache table
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS cache (
		url TEXT PRIMARY KEY,
		file_path TEXT,
		thumb_path TEXT,
		format TEXT,
		access_count INTEGER DEFAULT 1,
		last_access TIMESTAMP,
		created_at TIMESTAMP,
		file_size INTEGER DEFAULT 0,
		content_type TEXT DEFAULT '',
		width INTEGER DEFAULT 0,
		height INTEGER DEFAULT 0
	)`)
	if err != nil {
		log.Fatalf("Creating cache table failed: %v", err)
	}
	
	// 尝试添加缺失的列（兼容旧数据库）
	db.Exec(`ALTER TABLE cache ADD COLUMN file_size INTEGER DEFAULT 0`)
	db.Exec(`ALTER TABLE cache ADD COLUMN content_type TEXT DEFAULT ''`)
	db.Exec(`ALTER TABLE cache ADD COLUMN width INTEGER DEFAULT 0`)
	db.Exec(`ALTER TABLE cache ADD COLUMN height INTEGER DEFAULT 0`)

	// 	Create stats table
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

	// 初始化统计记录
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

	// 加载请求计数
	err = db.QueryRow("SELECT total_requests FROM stats WHERE id = 1").Scan(&requestCount)
	if err != nil {
		log.Printf("Querying total requests failed: %v，using default value 0", err)
		requestCount = 0
	}
	
	// 启动数据库健康检查
	go checkDBHealth()
}

// checkDBHealth 定期检查数据库健康状态
func checkDBHealth() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		if err := db.Ping(); err != nil {
			log.Printf("数据库连接失败，尝试重新连接: %v", err)
			reconnectDB()
		}
	}
}

// reconnectDB 重新连接数据库
func reconnectDB() {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	
	// 关闭旧连接
	if db != nil {
		db.Close()
	}
	
	// 重新打开连接
	var err error
	for retries := 0; retries < 5; retries++ {
		db, err = sql.Open("sqlite", "./imgproxy.db")
		if err == nil {
			// 重新设置数据库参数
			pragmas := []string{
				"PRAGMA journal_mode = WAL;",
				"PRAGMA synchronous = NORMAL;",
				"PRAGMA temp_store = MEMORY;",
				"PRAGMA busy_timeout = 10000;",
				"PRAGMA cache_size = -64000;",
				"PRAGMA mmap_size = 268435456;",
			}
			
			for _, pragma := range pragmas {
				db.Exec(pragma)
			}
			
			db.SetMaxOpenConns(25)
			db.SetMaxIdleConns(5)
			db.SetConnMaxLifetime(5 * time.Minute)
			
			log.Println("数据库重新连接成功")
			return
		}
		
		log.Printf("数据库重连失败 (尝试 %d/5): %v", retries+1, err)
		time.Sleep(time.Duration(retries+1) * time.Second)
	}
	
	log.Println("数据库重连失败，某些功能可能不可用")
}

// executeWithRetry 带重试的数据库执行
func executeWithRetry(query string, args ...interface{}) (sql.Result, error) {
	var result sql.Result
	var err error
	
	for retries := 0; retries < 3; retries++ {
		result, err = db.Exec(query, args...)
		if err == nil {
			return result, nil
		}
		
		// 如果是数据库锁定错误，重试
		if strings.Contains(err.Error(), "database is locked") || 
		   strings.Contains(err.Error(), "database table is locked") {
			time.Sleep(time.Duration(100*(retries+1)) * time.Millisecond)
			continue
		}
		
		// 其他错误直接返回
		return nil, err
	}
	
	return nil, err
}

// queryWithRetry 带重试的数据库查询
func queryWithRetry(query string, args ...interface{}) (*sql.Rows, error) {
	var rows *sql.Rows
	var err error
	
	for retries := 0; retries < 3; retries++ {
		rows, err = db.Query(query, args...)
		if err == nil {
			return rows, nil
		}
		
		// 如果是数据库锁定错误，重试
		if strings.Contains(err.Error(), "database is locked") || 
		   strings.Contains(err.Error(), "database table is locked") {
			time.Sleep(time.Duration(100*(retries+1)) * time.Millisecond)
			continue
		}
		
		// 其他错误直接返回
		return nil, err
	}
	
	return nil, err
}

// 定期清理过期的缓存文件
func cleanExpiredCache() {
	for {
		time.Sleep(6 * time.Hour) //  Expired cache every 6 hours
		log.Println("Starting to clean expired cache...")

		dbMutex.Lock()
		// 查询需要清理的缓存记录
		rows, err := queryWithRetry(`
			SELECT url, file_path, access_count, last_access FROM cache
			WHERE last_access < datetime('now', '-10 minutes')
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
			var lastAccess time.Time
			if err := rows.Scan(&url, &filePath, &accessCount, &lastAccess); err != nil {
				log.Printf("Reading cache record failed: %v", err)
				continue
			}

			// 统一缓存有效期为10分钟
			expireMinutes := 10

			// 检查是否真的过期
			expireTime := lastAccess.Add(time.Duration(expireMinutes) * time.Minute)
			if time.Now().After(expireTime) {
				expiredURLs = append(expiredURLs, url)
				expiredFiles = append(expiredFiles, filePath)
				count++
			}
		}
		rows.Close()

		// 	Deleting expired cache files
		for i, filePath := range expiredFiles {
			// 	Deleting cache file
			if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
				log.Printf("Deleting expired cache file failed %s: %v", filePath, err)
			}

			// 	Deleting cache record
			_, err := db.Exec("DELETE FROM cache WHERE url = ?", expiredURLs[i])
			if err != nil {
				log.Printf("Deleting cache record failed: %v", err)
			}
		}

		dbMutex.Unlock()
		log.Printf("Finished deleting %d expired cache files", count)
	}
}

// Generating cache file path
func getCacheFilePath(imageURL string, format string) string {
	// 	Generating cache file name
	// 	Using MD5 hash to create unique file name
	hasher := md5.New()
	hasher.Write([]byte(imageURL))
	hash := hex.EncodeToString(hasher.Sum(nil))

	// 	Determining file extension based on image format
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

// hashPassword 简单的密码哈希
func hashPassword(password string) string {
	hash := md5.Sum([]byte(password + "salt"))
	return hex.EncodeToString(hash[:])
}

// showLoginPage 显示登录页面
func showLoginPage(w http.ResponseWriter, errorMsg string) {
	html := `
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>登录 - 缓存管理</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            margin: 0;
            padding: 0;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            height: 100vh;
            display: flex;
            justify-content: center;
            align-items: center;
        }
        .login-container {
            background: white;
            border-radius: 10px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.2);
            padding: 40px;
            width: 100%;
            max-width: 400px;
        }
        h2 {
            margin: 0 0 30px;
            color: #333;
            text-align: center;
        }
        .form-group {
            margin-bottom: 20px;
        }
        label {
            display: block;
            margin-bottom: 5px;
            color: #666;
            font-size: 14px;
        }
        input[type="password"] {
            width: 100%;
            padding: 12px;
            border: 1px solid #ddd;
            border-radius: 5px;
            font-size: 16px;
            box-sizing: border-box;
        }
        input[type="password"]:focus {
            outline: none;
            border-color: #667eea;
        }
        button {
            width: 100%;
            padding: 12px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            border: none;
            border-radius: 5px;
            font-size: 16px;
            cursor: pointer;
            transition: transform 0.2s;
        }
        button:hover {
            transform: translateY(-2px);
        }
        .error {
            color: #dc3545;
            font-size: 14px;
            margin-top: 10px;
            text-align: center;
        }
    </style>
</head>
<body>
    <div class="login-container">
        <h2>🔐 缓存管理登录</h2>
        <form method="POST">
            <div class="form-group">
                <label for="password">管理员密码</label>
                <input type="password" id="password" name="password" required autofocus>
            </div>
            <button type="submit">登录</button>
            ` + (func() string {
		if errorMsg != "" {
			return `<div class="error">` + errorMsg + `</div>`
		}
		return ""
	})() + `
        </form>
    </div>
</body>
</html>`
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// detectImageFormat 检测图片格式
func detectImageFormat(data []byte) string {
	if len(data) < 12 {
		return ""
	}
	
	// WebP: RIFF....WEBP
	if bytes.HasPrefix(data, []byte("RIFF")) && bytes.Contains(data[:12], []byte("WEBP")) {
		return "webp"
	}
	
	// PNG: 89 50 4E 47 0D 0A 1A 0A
	if bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		return "png"
	}
	
	// JPEG: FF D8 FF
	if bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}) {
		return "jpeg"
	}
	
	// GIF: GIF87a or GIF89a
	if bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")) {
		return "gif"
	}
	
	return ""
}

// Updating cache record
func updateCacheRecord(url, filePath, thumbPath, format string, isHit bool, originalSize, compressedSize int64) {
	// 如果启用内存缓存，更新内存
	if useMemCache {
		if isHit {
			// 缓存命中，LRU的Get方法会自动更新访问信息
			lruCache.Get(url)
			
			// 更新统计
			atomic.AddInt64(&cacheHits, 1)
		} else {
			// 缓存未命中，添加新记录
			var fileSize int64
			if filePath != "" {
				if fileInfo, err := os.Stat(filePath); err == nil {
					fileSize = fileInfo.Size()
				}
			}
			
			entry := &CacheEntry{
				URL:         url,
				FilePath:    filePath,
				ThumbPath:   thumbPath,
				Format:      format,
				AccessCount: 1,
				LastAccess:  time.Now(),
				CreatedAt:   time.Now(),
				Dirty:       true,
				Size:        fileSize,
			}
			lruCache.Put(url, entry)
			
			// 更新统计
			atomic.AddInt64(&cacheMisses, 1)
		}
		
		return
	}
	
	// 直接更新数据库（内存缓存禁用时）
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if isHit {
		// 	Updating cache record when cache hit
		_, err := db.Exec(
			"UPDATE cache SET access_count = access_count + 1, last_access = datetime('now') WHERE url = ?",
			url,
		)
		if err != nil {
			log.Printf("Updating cache record failed: %v", err)
		}

		// 	Updating cache hit statistics
		_, err = db.Exec("UPDATE stats SET total_cache_hits = total_cache_hits + 1 WHERE id = 1")
		if err != nil {
			log.Printf("Updating cache hit statistics failed: %v", err)
		}

		// 	Updating cache hit statistics
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
		// 	Updating cache miss statistics
		_, err := db.Exec(
			"INSERT INTO cache (url, file_path, thumb_path, format, access_count, last_access, created_at) VALUES (?, ?, ?, ?, 1, datetime('now'), datetime('now'))",
			url, filePath, thumbPath, format,
		)
		if err != nil {
			log.Printf("Updating cache miss statistics failed: %v", err)
		}

		// 	Updating cache miss statistics
		_, err = db.Exec("UPDATE stats SET total_cache_misses = total_cache_misses + 1 WHERE id = 1")
		if err != nil {
			log.Printf("Updating cache miss statistics failed: %v", err)
		}

		// 	Updating cache miss statistics
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

	// 	Updating total requests statistics
	_, err := db.Exec("UPDATE stats SET total_requests = ? WHERE id = 1", atomic.LoadInt64(&requestCount))
	if err != nil {
		log.Printf("Updating total requests statistics failed: %v", err)
	}
}

// From cache getting image
func getFromCache(imageURL string) ([]byte, string, bool) {
	// 如果启用内存缓存，先从LRU缓存查找
	if useMemCache {
		entry, exists := lruCache.Get(imageURL)
		
		if exists {
			// 检查是否过期
			cacheValidity := time.Duration(cacheConfig.CacheValidityMin) * time.Minute
			if time.Since(entry.LastAccess) > cacheValidity {
				// 过期了，删除
				lruCache.Remove(imageURL)
				return nil, "", false
			}
			
			// 读取文件
			imgData, err := os.ReadFile(entry.FilePath)
			if err != nil {
				log.Printf("Reading cache file failed: %v", err)
				// 文件不存在，删除缓存
				if os.IsNotExist(err) {
					lruCache.Remove(imageURL)
				}
				return nil, "", false
			}
			
			// 访问信息已在Get方法中更新
			return imgData, entry.Format, true
		}
	}
	
	// 从数据库查询（向后兼容或内存缓存禁用时）
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

	// 	Reading cache file
	imgData, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("Reading cache file failed: %v", err)
		// 	Deleting cache file
		if os.IsNotExist(err) {
			_, _ = db.Exec("DELETE FROM cache WHERE url = ?", imageURL)
		}
		return nil, "", false
	}

	return imgData, format, true
}

func handleImageProxy(w http.ResponseWriter, r *http.Request) {
	// 支持三种方式传递URL：
	// 1. 查询参数方式（推荐，可以保留双斜杠）: /?url=https://example.com//path//to//image.jpg
	// 2. 编码路径方式（使用_DS_代替//）: /https:_DS_example.com_DS_path_DS_to_DS_image.jpg
	// 3. 标准路径方式（兼容旧版本）: /https://example.com/path/to/image.jpg
	
	imageURL := r.URL.Query().Get("url")
	
	// 如果没有使用查询参数，则使用路径方式（向后兼容）
	if imageURL == "" {
		if r.URL.Path == "/" || r.URL.Path == "/favicon.ico" {
			// 如果是根路径，显示使用说明
			if r.URL.Path == "/" && imageURL == "" {
				// 获取当前访问的主机名
				scheme := "http"
				if r.TLS != nil {
					scheme = "https"
				}
				host := r.Host
				if host == "" {
					host = "localhost:8080"
				}
				baseURL := fmt.Sprintf("%s://%s", scheme, host)
				
				// 获取语言设置
				lang := getLang(r)
				langCode := "zh"
				if cookie, err := r.Cookie("lang"); err == nil {
					langCode = cookie.Value
				}
				
				// 设置语言切换按钮的active类
				zhActive := ""
				enActive := ""
				if langCode == "zh" {
					zhActive = "active"
				} else {
					enActive = "active"
				}
				
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				helpHTML := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <title>WebP %s</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif; max-width: 900px; margin: 30px auto; padding: 20px; background: #f5f5f5; }
        .container { background: white; border-radius: 10px; padding: 30px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h1 { color: #333; margin-bottom: 10px; }
        h2 { color: #555; margin-top: 30px; margin-bottom: 15px; border-bottom: 2px solid #4CAF50; padding-bottom: 5px; }
        pre { background: #f8f9fa; padding: 12px; border-radius: 5px; overflow-x: auto; border: 1px solid #e0e0e0; font-size: 13px; }
        .example { margin: 20px 0; }
        .example h3 { color: #666; font-size: 14px; margin-bottom: 8px; }
        .example p { color: #888; font-size: 12px; margin-top: 5px; }
        .lang-switcher { position: absolute; top: 20px; right: 30px; }
        .lang-switcher a { text-decoration: none; padding: 5px 15px; background: #4CAF50; color: white; border-radius: 5px; margin-left: 5px; font-size: 14px; }
        .lang-switcher a:hover { background: #45a049; }
        .lang-switcher a.active { background: #333; }
        .management-links { margin-top: 30px; padding: 20px; background: #f0f8ff; border-radius: 5px; }
        .management-links h2 { margin-top: 0; border-color: #2196F3; }
        .management-links ul { list-style: none; padding: 0; }
        .management-links li { margin: 10px 0; }
        .management-links a { color: #2196F3; text-decoration: none; font-weight: 500; }
        .management-links a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <div class="lang-switcher">
        <a href="javascript:void(0)" onclick="switchLang('zh')" class="%s">中文</a>
        <a href="javascript:void(0)" onclick="switchLang('en')" class="%s">English</a>
    </div>
    
    <div class="container">
        <h1>WebP %s</h1>
        <h2>%s</h2>
        
        <div class="example">
            <h3>1. %s</h3>
            <pre>%s/?url=https://example.com//path//to//image.jpg</pre>
        </div>
        
        <div class="example">
            <h3>2. %s</h3>
            <pre>%s/https:_DS_example.com_DS_path_DS_to_DS_image.jpg</pre>
        </div>
        
        <div class="example">
            <h3>3. %s</h3>
            <pre>%s/https://example.com/path/to/image.jpg</pre>
        </div>
        
        <h2>%s</h2>
        <div class="example">
            <h3>%s</h3>
            <pre>%s/?url=https://example.com/image.png&format=webp</pre>
        </div>
        
        <div class="example">
            <h3>%s</h3>
            <pre>%s/?url=https://example.com/image.png&format=original</pre>
        </div>
        
        <h2>%s</h2>
        <div class="example">
            <h3>%s</h3>
            <pre>%s/?url=https://example.com/image.jpg&w=500</pre>
        </div>
        
        <div class="example">
            <h3>%s</h3>
            <pre>%s/?url=https://example.com/image.jpg&h=300</pre>
        </div>
        
        <div class="example">
            <h3>%s</h3>
            <pre>%s/?url=https://example.com/image.jpg&w=500&h=300</pre>
        </div>
        
        <div class="example">
            <h3>%s</h3>
            <pre>%s/?url=https://example.com/image.jpg&w=800&format=webp&q=90</pre>
        </div>
        
        <h2>%s</h2>
        <div class="example">
            <h3>fit %s</h3>
            <pre>%s/?url=https://example.com/image.jpg&w=500&h=300&mode=fit</pre>
            <p>%s</p>
        </div>
        
        <div class="example">
            <h3>fill - %s</h3>
            <pre>%s/?url=https://example.com/image.jpg&w=500&h=300&mode=fill</pre>
            <p>%s</p>
        </div>
        
        <div class="example">
            <h3>stretch - %s</h3>
            <pre>%s/?url=https://example.com/image.jpg&w=500&h=300&mode=stretch</pre>
            <p>%s</p>
        </div>
        
        <div class="example">
            <h3>pad - %s</h3>
            <pre>%s/?url=https://example.com/image.jpg&w=500&h=300&mode=pad</pre>
            <p>%s</p>
        </div>
        
        <div class="management-links">
            <h2>%s</h2>
            <ul>
                <li><a href="/cache">%s</a></li>
                <li><a href="/stats">%s</a></li>
                <li><a href="/upload">%s</a></li>
            </ul>
            <p style="margin-top: 15px; color: #666; font-size: 12px;">%s <a href="https://github.com/zots0127/io" target="_blank" style="color: #2196F3;">github.com/zots0127/io</a></p>
        </div>
    </div>
    
    <script>
    function switchLang(lang) {
        document.cookie = 'lang=' + lang + '; path=/; max-age=2592000';
        location.reload();
    }
    </script>
</body>
</html>`,
    lang.UI["service_title"],
    zhActive,
    enActive,
    lang.UI["service_title"],
    lang.UI["usage_title"],
    lang.UI["query_param_method"], baseURL,
    lang.UI["encoded_path_method"], baseURL,
    lang.UI["standard_path_method"], baseURL,
    lang.UI["format_conversion_title"],
    lang.UI["force_webp_conversion"], baseURL,
    lang.UI["keep_original_format"], baseURL,
    lang.UI["image_resize_title"],
    lang.UI["specify_width"], baseURL,
    lang.UI["specify_height"], baseURL,
    lang.UI["specify_both_dimensions"], baseURL,
    lang.UI["combined_params"], baseURL,
    lang.UI["resize_mode_title"],
    lang.UI["mode_fit_default"], baseURL, lang.UI["mode_fit_desc"],
    lang.UI["mode_fill"], baseURL, lang.UI["mode_fill_desc"],
    lang.UI["mode_stretch"], baseURL, lang.UI["mode_stretch_desc"],
    lang.UI["mode_pad"], baseURL, lang.UI["mode_pad_desc"],
    lang.UI["management_pages_title"],
    lang.UI["cache_management"],
    lang.UI["statistics_json"],
    lang.UI["image_upload"],
    lang.UI["backend_note"])
				w.Write([]byte(helpHTML))
				return
			}
			http.NotFound(w, r)
			return
		}
		
		imageURL = strings.TrimPrefix(r.URL.Path, "/")
		
		// 检查是否使用了 _DS_ 编码（代表双斜杠）
		if strings.Contains(imageURL, "_DS_") {
			// 将 _DS_ 替换回 //
			imageURL = strings.ReplaceAll(imageURL, "_DS_", "//")
		}
		
		if imageURL == "" {
			http.Error(w, "未指定图片URL", http.StatusBadRequest)
			return
		}
	}

	// 	Checking URL format
	if !strings.HasPrefix(imageURL, "http://") && !strings.HasPrefix(imageURL, "https://") {
		// 	Fixing missing colon case, such as https//example.com
		if strings.HasPrefix(imageURL, "http/") {
			imageURL = strings.Replace(imageURL, "http/", "http:/", 1)
		} else if strings.HasPrefix(imageURL, "https/") {
			imageURL = strings.Replace(imageURL, "https/", "https:/", 1)
		}

		// 	Fixing URL format
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
	
	// 处理URL参数分离
	// 如果使用 ?url= 方式，原始URL参数保持不变，代理参数从r.URL.Query()获取
	// 如果使用路径方式，且URL包含参数，需要智能分离
	cleanImageURL := imageURL
	
	// 只有当不是通过 ?url= 参数传递时，才需要从原始URL中分离代理参数
	if r.URL.Query().Get("url") == "" && parsedURL.RawQuery != "" {
		// 路径方式，检查是否有代理参数混在原始URL中
		originalQuery := parsedURL.Query()
		cleanedQuery := url.Values{}
		proxyParams := map[string]bool{
			"format": true,
			"w":      true,
			"h":      true,
			"q":      true,
			"mode":   true,
		}
		
		// 遍历所有参数，只保留非代理参数
		for key, values := range originalQuery {
			// 如果这个参数同时存在于r.URL.Query()中，说明是代理参数
			if _, isProxyParam := proxyParams[key]; isProxyParam && r.URL.Query().Get(key) != "" {
				// 这是代理参数，不包含在清理后的URL中
				continue
			}
			// 保留原始参数
			for _, value := range values {
				cleanedQuery.Add(key, value)
			}
		}
		
		parsedURL.RawQuery = cleanedQuery.Encode()
		cleanImageURL = parsedURL.String()
	}

	// 获取格式参数（如果指定）
	requestedFormat := r.URL.Query().Get("format")
	forceWebP := false
	forceOriginal := false
	
	if requestedFormat != "" {
		requestedFormat = strings.ToLower(requestedFormat)
		// 验证请求的格式
		switch requestedFormat {
		case "webp":
			forceWebP = true
		case "original":
			forceOriginal = true
		case "png", "jpeg", "jpg", "gif":
			// 这些格式暂时当作 original 处理
			forceOriginal = true
		default:
			http.Error(w, "不支持的格式。支持的格式: webp, original", http.StatusBadRequest)
			return
		}
	}

	// 获取尺寸参数
	widthStr := r.URL.Query().Get("w")
	heightStr := r.URL.Query().Get("h")
	qualityStr := r.URL.Query().Get("q")
	modeStr := r.URL.Query().Get("mode")
	
	var targetWidth, targetHeight int
	var quality int = 80 // 默认质量
	var resizeMode string = "fit" // 默认模式
	
	if widthStr != "" {
		if width, err := strconv.Atoi(widthStr); err == nil && width > 0 && width <= 5000 {
			targetWidth = width
		} else {
			http.Error(w, "宽度参数无效，必须是 1-5000 之间的整数", http.StatusBadRequest)
			return
		}
	}
	
	if heightStr != "" {
		if height, err := strconv.Atoi(heightStr); err == nil && height > 0 && height <= 5000 {
			targetHeight = height
		} else {
			http.Error(w, "高度参数无效，必须是 1-5000 之间的整数", http.StatusBadRequest)
			return
		}
	}
	
	if qualityStr != "" {
		if q, err := strconv.Atoi(qualityStr); err == nil && q >= 1 && q <= 100 {
			quality = q
		} else {
			http.Error(w, "质量参数无效，必须是 1-100 之间的整数", http.StatusBadRequest)
			return
		}
	}
	
	if modeStr != "" {
		validModes := map[string]bool{
			"fit": true,     // 适应框内，保持纵横比（默认）
			"fill": true,    // 填充整个框，裁剪多余部分
			"stretch": true, // 拉伸到精确尺寸，可能变形
			"pad": true,     // 适应框内并添加白色边距
		}
		if validModes[modeStr] {
			resizeMode = modeStr
		} else {
			http.Error(w, "模式参数无效。支持的模式: fit, fill, stretch, pad", http.StatusBadRequest)
			return
		}
	}

	// 根据参数生成缓存键
	// 使用清理后的URL作为基础，确保缓存键的一致性
	cacheKey := cleanImageURL
	params := []string{}
	
	if forceWebP {
		params = append(params, "format=webp")
	} else if forceOriginal {
		params = append(params, "format=original")
	}
	
	if targetWidth > 0 {
		params = append(params, fmt.Sprintf("w=%d", targetWidth))
	}
	if targetHeight > 0 {
		params = append(params, fmt.Sprintf("h=%d", targetHeight))
	}
	if quality != 80 {
		params = append(params, fmt.Sprintf("q=%d", quality))
	}
	if resizeMode != "fit" && (targetWidth > 0 || targetHeight > 0) {
		params = append(params, fmt.Sprintf("mode=%s", resizeMode))
	}
	
	if len(params) > 0 {
		cacheKey = imageURL + "?" + strings.Join(params, "&")
	}

	// 	From cache getting image
	imgData, format, cacheHit := getFromCache(cacheKey)

	// 	Checking cache hit
	if !cacheHit {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(cleanImageURL)
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

		// 使用缓冲池读取原始图片数据
		buf := largeBufferPool.Get().(*bytes.Buffer)
		buf.Reset()
		defer largeBufferPool.Put(buf)
		
		_, err = io.Copy(buf, resp.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("读取图片数据失败: %v", err), http.StatusInternalServerError)
			return
		}
		rawImgData := buf.Bytes()

		// 检测图片格式
		detectedFormat := detectImageFormat(rawImgData)
		var img image.Image
		
		// 特殊处理 WebP 格式
		if detectedFormat == "webp" {
			// 对于 WebP 输入，如果是原始格式或 WebP 输出，直接传递
			// 否则，由于我们没有 WebP 解码器，报错
			if forceOriginal || forceWebP || requestedFormat == "" {
				// 默认行为或强制 WebP/原始，直接使用原始数据
				format = "webp"
				img = nil // 不需要解码
			} else {
				// 需要转换为其他格式，但我们无法解码 WebP
				http.Error(w, "无法解码 WebP 格式的图片。请使用 format=original 或 format=webp 参数", http.StatusUnsupportedMediaType)
				return
			}
		} else {
			// 使用标准库解码其他格式
			img, detectedFormat, err = image.Decode(bytes.NewReader(rawImgData))
			if err != nil {
				http.Error(w, fmt.Sprintf("图片解码失败: %v", err), http.StatusUnsupportedMediaType)
				return
			}
			format = detectedFormat
		}
		
		// 如果需要调整尺寸并且有图片对象
		needResize := (targetWidth > 0 || targetHeight > 0) && img != nil
		if needResize {
			img = resizeImage(img, targetWidth, targetHeight, resizeMode)
		}
		
		// 使用新的缓冲区用于输出
		outputBuf := largeBufferPool.Get().(*bytes.Buffer)
		outputBuf.Reset()
		defer largeBufferPool.Put(outputBuf)

		// 根据参数决定输出格式
		if forceOriginal && !needResize {
			// 保持原始格式且不需要缩放
			format = detectedFormat
			outputBuf.Write(rawImgData)
		} else if forceWebP {
			// 强制转换为 WebP
			format = "webp"
			if detectedFormat == "webp" && !needResize {
				// 如果原始就是 WebP 且不需要缩放，直接使用
				outputBuf.Write(rawImgData)
			} else if img != nil {
				// 需要转换为 WebP 或需要缩放
				if err := nativewebp.Encode(outputBuf, img, nil); err != nil {
					http.Error(w, fmt.Sprintf("WebP 编码失败: %v", err), http.StatusInternalServerError)
					return
				}
			} else {
				// img 为 nil 但需要 WebP，使用原始数据
				outputBuf.Write(rawImgData)
			}
		} else if forceOriginal && needResize {
			// 保持原始格式但需要缩放
			// 只有当我们能够编码回原始格式时才能处理
			if img != nil {
				format = detectedFormat
				// 目前只能输出 WebP，所以转换为 WebP
				format = "webp"
				if err := nativewebp.Encode(outputBuf, img, nil); err != nil {
					http.Error(w, fmt.Sprintf("WebP 编码失败: %v", err), http.StatusInternalServerError)
					return
				}
			} else {
				http.Error(w, "无法缩放此格式的图片", http.StatusInternalServerError)
				return
			}
		} else {
			// 默认行为
			if detectedFormat == "webp" && !needResize {
				// WebP 输入，保持 WebP，不需要缩放
				format = "webp"
				outputBuf.Write(rawImgData)
			} else if detectedFormat == "webp" && needResize {
				// WebP 输入但需要缩放，因为无法解码WebP，报错
				http.Error(w, "无法缩放 WebP 格式的图片", http.StatusInternalServerError)
				return
			} else if format == "gif" {
				// GIF 格式
				if needResize {
					// GIF 需要缩放，只能处理为静态 WebP
					format = "webp"
					if img != nil {
						if err := nativewebp.Encode(outputBuf, img, nil); err != nil {
							http.Error(w, fmt.Sprintf("WebP 编码失败: %v", err), http.StatusInternalServerError)
							return
						}
					}
				} else {
					// 不需要缩放，检查是否为动态GIF
					gifImg, err := gif.DecodeAll(bytes.NewReader(rawImgData))
					if err != nil || len(gifImg.Image) <= 1 {
						// 静态GIF或解码失败，转为静态WebP
						format = "webp"
						if img != nil {
							if err := nativewebp.Encode(outputBuf, img, nil); err != nil {
								http.Error(w, fmt.Sprintf("WebP 编码失败: %v", err), http.StatusInternalServerError)
								return
							}
						} else {
							outputBuf.Write(rawImgData)
						}
					} else {
						// 动态GIF保持原格式
						format = "gif"
						if err := gif.EncodeAll(outputBuf, gifImg); err != nil {
							http.Error(w, fmt.Sprintf("GIF 编码失败: %v", err), http.StatusInternalServerError)
							return
						}
					}
				}
			} else {
				// 所有其他格式（PNG、JPEG等）都转换为静态WebP
				format = "webp"
				if img != nil {
					if err := nativewebp.Encode(outputBuf, img, nil); err != nil {
						http.Error(w, fmt.Sprintf("WebP 编码失败: %v", err), http.StatusInternalServerError)
						return
					}
				} else {
					// 如果无法解码但是原始格式，使用原始数据
					outputBuf.Write(rawImgData)
				}
			}
		}

		// 保存到缓存
		imgData = outputBuf.Bytes()
		originalSize := int64(len(rawImgData))
		compressedSize := int64(len(imgData))
		cachePath := getCacheFilePath(cacheKey, format)

		// 生成缩略图
		thumbPath := ""
		if img != nil {
			thumb := generateThumbnail(img, 200, 200)
			if thumb != nil {
				var thumbBuf bytes.Buffer
				if err := nativewebp.Encode(&thumbBuf, thumb, nil); err == nil {
					thumbFileName := strings.TrimSuffix(filepath.Base(cachePath), filepath.Ext(cachePath)) + "_thumb.webp"
					thumbPath = filepath.Join(cacheDir, "thumbs", thumbFileName)
					if err := os.WriteFile(thumbPath, thumbBuf.Bytes(), 0644); err != nil {
						log.Printf("保存缩略图失败: %v", err)
						thumbPath = "" // 重置为空
					}
				} else {
					log.Printf("缩略图编码失败: %v", err)
				}
			}
		}

		if err := os.WriteFile(cachePath, imgData, 0644); err != nil {
			log.Printf("保存缓存失败: %v", err)
			// 继续处理，即使缓存失败
		} else {
			// 更新数据库记录
			updateCacheRecord(cacheKey, cachePath, thumbPath, format, false, originalSize, compressedSize)
		}
	} else {
		// 缓存命中，更新记录
		// 对于缓存命中，我们假设平均压缩比来估算原始大小
		compressedSize := int64(len(imgData))
		estimatedOriginalSize := compressedSize * 3 // 假设平均压缩比为3:1
		updateCacheRecord(cacheKey, "", "", format, true, estimatedOriginalSize, compressedSize)
	}

	// 生成并检查 ETag
	etag := generateETag(imgData)
	w.Header().Set("ETag", etag)
	
	// 检查客户端缓存
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	
	// 设置适当的Content-Type
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

	// 设置缓存控制头
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
	// 如果启用内存缓存，使用内存中的统计
	if useMemCache {
		totalHits = int(atomic.LoadInt64(&cacheHits))
		totalMisses = int(atomic.LoadInt64(&cacheMisses))
		// 仍然从数据库获取节省的空间和流量（这些在syncToDB时更新）
		db.QueryRow("SELECT total_bytes_saved, total_bandwidth_saved FROM stats WHERE id = 1").Scan(&totalBytesSaved, &totalBandwidthSaved)
	} else {
		err := db.QueryRow("SELECT total_cache_hits, total_cache_misses, total_bytes_saved, total_bandwidth_saved FROM stats WHERE id = 1").Scan(&totalHits, &totalMisses, &totalBytesSaved, &totalBandwidthSaved)
		if err != nil {
			log.Printf("获取缓存统计失败: %v", err)
			totalHits = 0
			totalMisses = 0
			totalBytesSaved = 0
			totalBandwidthSaved = 0
		}
	}

	// 获取缓存文件数量
	err := db.QueryRow("SELECT COUNT(*) FROM cache").Scan(&cacheFiles)
	if err != nil {
		log.Printf("获取缓存文件数量失败: %v", err)
		cacheFiles = 0
	}

	// 获取缓存大小
	rows, err := queryWithRetry("SELECT file_path FROM cache")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var filePath string
			if err := rows.Scan(&filePath); err != nil {
				continue
			}
			if info, err := os.Stat(filePath); err == nil {
				cacheSize += info.Size()
			}
		}
	}
	dbMutex.Unlock()

	cacheSizeMB := float64(cacheSize) / 1024 / 1024
	hitRate := 0.0
	if totalHits+totalMisses > 0 {
		hitRate = float64(totalHits) * 100 / float64(totalHits+totalMisses)
	}

	// 计算节省的空间和流量（MB）
	bytesSavedMB := float64(totalBytesSaved) / 1024 / 1024
	bandwidthSavedMB := float64(totalBandwidthSaved) / 1024 / 1024

	// 获取内存缓存信息
	memCacheEntries := 0
	memCacheEstSize := int64(0)
	if useMemCache {
		memCacheEntries = lruCache.Len()
		memCacheEstSize = lruCache.currentSize
	}
	
	// 获取当前访问的主机名
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost:8080"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, host)
	
	// 构建 JSON 响应
	stats := map[string]interface{}{
		"request_stats": map[string]interface{}{
			"total_requests": count,
			"current_time":   time.Now().Format("2006-01-02 15:04:05"),
		},
		"cache_stats": map[string]interface{}{
			"file_count": cacheFiles,
			"size_mb":    math.Round(cacheSizeMB*100) / 100, // 保留两位小数
			"hits":       totalHits,
			"misses":     totalMisses,
			"hit_rate":   math.Round(hitRate*10) / 10, // 保留一位小数
		},
		"memory_cache": map[string]interface{}{
			"enabled":           useMemCache,
			"entries":           memCacheEntries,
			"estimated_size_mb": math.Round(float64(memCacheEstSize)/(1024*1024)*100) / 100,
			"max_entries":       cacheConfig.MaxMemCacheEntries,
			"max_size_mb":       cacheConfig.MaxMemCacheSizeMB,
			"cleanup_interval":  fmt.Sprintf("%dm", cacheConfig.CleanupIntervalMin),
			"access_window":     fmt.Sprintf("%dm", cacheConfig.AccessWindowMin),
		},
		"savings_stats": map[string]interface{}{
			"total_space_saved_mb":     math.Round(bytesSavedMB*100) / 100,     // 总节省空间(MB)
			"total_bandwidth_saved_mb": math.Round(bandwidthSavedMB*100) / 100, // 总节省流量(MB)
			"compression_efficiency":   "WebP格式平均节省60-80%空间",
		},
		"cache_rules": map[string]string{
			"cache_duration": "10分钟",
			"note":           "所有缓存文件统一有效期10分钟，从最后一次访问时间开始计算",
		},
		"usage": fmt.Sprintf("%s/https://example.com/image.jpg", baseURL),
	}

	jsonData, err := json.Marshal(stats)
	if err != nil {
		http.Error(w, "生成JSON失败", http.StatusInternalServerError)
		return
	}

	w.Write(jsonData)
}

// 生成缩略图
// resizeImage 调整图片大小，支持多种缩放模式
func resizeImage(img image.Image, targetWidth, targetHeight int, mode string) image.Image {
	if img == nil {
		return nil
	}
	
	bounds := img.Bounds()
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()
	
	// 如果没有指定尺寸，返回原图
	if targetWidth == 0 && targetHeight == 0 {
		return img
	}
	
	// 处理只指定一个维度的情况
	if targetWidth == 0 {
		// 只指定高度，按比例计算宽度
		targetWidth = int(float64(origWidth) * float64(targetHeight) / float64(origHeight))
	} else if targetHeight == 0 {
		// 只指定宽度，按比例计算高度
		targetHeight = int(float64(origHeight) * float64(targetWidth) / float64(origWidth))
	}
	
	var result image.Image
	
	switch mode {
	case "stretch":
		// 拉伸模式：直接缩放到目标尺寸，可能变形
		result = scaleImage(img, targetWidth, targetHeight)
		
	case "fill":
		// 填充模式：缩放并裁剪，确保填满整个框
		scaleX := float64(targetWidth) / float64(origWidth)
		scaleY := float64(targetHeight) / float64(origHeight)
		scale := math.Max(scaleX, scaleY) // 使用较大的缩放比例
		
		scaledWidth := int(float64(origWidth) * scale)
		scaledHeight := int(float64(origHeight) * scale)
		
		// 先缩放
		scaled := scaleImage(img, scaledWidth, scaledHeight)
		
		// 然后裁剪中心部分
		cropX := (scaledWidth - targetWidth) / 2
		cropY := (scaledHeight - targetHeight) / 2
		result = cropImage(scaled, cropX, cropY, targetWidth, targetHeight)
		
	case "pad":
		// 边距模式：缩放后添加白色边距
		scaleX := float64(targetWidth) / float64(origWidth)
		scaleY := float64(targetHeight) / float64(origHeight)
		scale := math.Min(scaleX, scaleY) // 使用较小的缩放比例
		
		scaledWidth := int(float64(origWidth) * scale)
		scaledHeight := int(float64(origHeight) * scale)
		
		// 先缩放
		scaled := scaleImage(img, scaledWidth, scaledHeight)
		
		// 创建带白色背景的目标图片
		padded := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
		// 填充白色背景
		for y := 0; y < targetHeight; y++ {
			for x := 0; x < targetWidth; x++ {
				padded.Set(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
		
		// 将缩放后的图片居中放置
		offsetX := (targetWidth - scaledWidth) / 2
		offsetY := (targetHeight - scaledHeight) / 2
		for y := 0; y < scaledHeight; y++ {
			for x := 0; x < scaledWidth; x++ {
				padded.Set(x+offsetX, y+offsetY, scaled.At(x, y))
			}
		}
		result = padded
		
	default: // "fit"
		// 适应模式：保持纵横比，适应框内（默认）
		scaleX := float64(targetWidth) / float64(origWidth)
		scaleY := float64(targetHeight) / float64(origHeight)
		scale := math.Min(scaleX, scaleY) // 使用较小的缩放比例
		
		newWidth := int(float64(origWidth) * scale)
		newHeight := int(float64(origHeight) * scale)
		result = scaleImage(img, newWidth, newHeight)
	}
	
	return result
}

// scaleImage 执行实际的图片缩放（双线性插值）
func scaleImage(img image.Image, newWidth, newHeight int) image.Image {
	if img == nil {
		return nil
	}
	
	bounds := img.Bounds()
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()
	
	// 创建新图片
	resized := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	
	// 使用双线性插值进行缩放
	scaleX := float64(origWidth) / float64(newWidth)
	scaleY := float64(origHeight) / float64(newHeight)
	
	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			srcX := float64(x) * scaleX
			srcY := float64(y) * scaleY
			
			x0 := int(srcX)
			y0 := int(srcY)
			x1 := x0 + 1
			y1 := y0 + 1
			
			if x1 >= origWidth {
				x1 = origWidth - 1
			}
			if y1 >= origHeight {
				y1 = origHeight - 1
			}
			
			fx := srcX - float64(x0)
			fy := srcY - float64(y0)
			
			// 双线性插值
			c00 := img.At(x0, y0)
			c10 := img.At(x1, y0)
			c01 := img.At(x0, y1)
			c11 := img.At(x1, y1)
			
			r00, g00, b00, a00 := c00.RGBA()
			r10, g10, b10, a10 := c10.RGBA()
			r01, g01, b01, a01 := c01.RGBA()
			r11, g11, b11, a11 := c11.RGBA()
			
			r := uint32((1-fx)*(1-fy)*float64(r00) + fx*(1-fy)*float64(r10) + 
			            (1-fx)*fy*float64(r01) + fx*fy*float64(r11))
			g := uint32((1-fx)*(1-fy)*float64(g00) + fx*(1-fy)*float64(g10) + 
			            (1-fx)*fy*float64(g01) + fx*fy*float64(g11))
			b := uint32((1-fx)*(1-fy)*float64(b00) + fx*(1-fy)*float64(b10) + 
			            (1-fx)*fy*float64(b01) + fx*fy*float64(b11))
			a := uint32((1-fx)*(1-fy)*float64(a00) + fx*(1-fy)*float64(a10) + 
			            (1-fx)*fy*float64(a01) + fx*fy*float64(a11))
			
			resized.Set(x, y, color.RGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(b >> 8),
				A: uint8(a >> 8),
			})
		}
	}
	
	return resized
}

// cropImage 裁剪图片
func cropImage(img image.Image, x, y, width, height int) image.Image {
	if img == nil {
		return nil
	}
	
	// 创建裁剪后的图片
	cropped := image.NewRGBA(image.Rect(0, 0, width, height))
	
	// 复制像素
	for dy := 0; dy < height; dy++ {
		for dx := 0; dx < width; dx++ {
			srcX := x + dx
			srcY := y + dy
			// 确保不越界
			if srcX >= 0 && srcY >= 0 && srcX < img.Bounds().Dx() && srcY < img.Bounds().Dy() {
				cropped.Set(dx, dy, img.At(srcX, srcY))
			}
		}
	}
	
	return cropped
}

func generateThumbnail(img image.Image, maxWidth, maxHeight int) image.Image {
	bounds := img.Bounds()
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()

	// 计算缩放比例
	scaleX := float64(maxWidth) / float64(origWidth)
	scaleY := float64(maxHeight) / float64(origHeight)
	scale := math.Min(scaleX, scaleY)

	// 如果图片已经足够小，直接返回
	if scale >= 1.0 {
		return img
	}

	// 计算新尺寸
	newWidth := int(float64(origWidth) * scale)
	newHeight := int(float64(origHeight) * scale)

	// 创建新图片
	thumbnail := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	// 简单的最近邻缩放
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

// 处理缩略图请求
func handleThumbnail(w http.ResponseWriter, r *http.Request) {
	// 从URL路径中提取文件名
	fileName := strings.TrimPrefix(r.URL.Path, "/thumb/")
	if fileName == "" {
		http.Error(w, "缺少文件名", http.StatusBadRequest)
		return
	}

	// 构建缩略图文件路径
	thumbPath := filepath.Join(cacheDir, "thumbs", fileName)

	// 检查缩略图文件是否存在
	if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
		http.Error(w, "缩略图不存在", http.StatusNotFound)
		return
	}

	// 读取并返回缩略图
	thumbData, err := os.ReadFile(thumbPath)
	if err != nil {
		http.Error(w, "读取缩略图失败", http.StatusInternalServerError)
		return
	}

	// 生成并检查 ETag
	etag := generateETag(thumbData)
	w.Header().Set("ETag", etag)
	
	// 检查客户端缓存
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	
	// 设置正确的Content-Type
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Cache-Control", "public, max-age=86400") // 缓存1天
	w.Write(thumbData)
}

// 缓存列表页面数据结构
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

// 处理缓存控制API
func handleCacheControl(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Query().Get("action")
	switch action {
	case "status":
		// GET 请求获取状态
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"enabled": useMemCache})
			return
		}
	case "toggle":
		// POST 请求切换状态
		if r.Method == "POST" {
			useMemCache = !useMemCache
			if useMemCache {
				loadCacheFromDB()
				go syncMemCacheToDB()
				go cleanupMemCache()
			} else {
				syncToDB() // 立即同步
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"enabled": useMemCache})
			return
		}
	case "sync":
		// POST 请求同步数据
		if r.Method == "POST" {
			syncToDB()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "synced"})
			return
		}
	case "lang":
		// 切换语言
		if r.Method == "POST" {
			var data map[string]string
			if err := json.NewDecoder(r.Body).Decode(&data); err == nil {
				if lang := data["lang"]; lang == "zh" || lang == "en" {
					// 设置cookie
					http.SetCookie(w, &http.Cookie{
						Name:     "lang",
						Value:    lang,
						Path:     "/",
						MaxAge:   86400 * 30, // 30天
						HttpOnly: false,
					})
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]string{"status": "ok", "lang": lang})
					return
				}
			}
			http.Error(w, "Invalid language", http.StatusBadRequest)
			return
		}
	case "config":
		// GET 请求获取配置
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cacheConfig)
			return
		}
		// POST 请求更新配置
		if r.Method == "POST" {
			var newConfig CacheConfig
			if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
				http.Error(w, "无效的配置数据", http.StatusBadRequest)
				return
			}
			
			// 验证配置的合理性
			if newConfig.MaxMemCacheEntries <= 0 || newConfig.MaxMemCacheEntries > 10000 {
				http.Error(w, "内存缓存条目数必须在1-10000之间", http.StatusBadRequest)
				return
			}
			if newConfig.MaxMemCacheSizeMB <= 0 || newConfig.MaxMemCacheSizeMB > 1000 {
				http.Error(w, "内存缓存大小必须在1-1000MB之间", http.StatusBadRequest)
				return
			}
			if newConfig.MaxDiskCacheSizeMB <= 0 || newConfig.MaxDiskCacheSizeMB > 10000 {
				http.Error(w, "磁盘缓存大小必须在1-10000MB之间", http.StatusBadRequest)
				return
			}
			if newConfig.CleanupIntervalMin <= 0 || newConfig.CleanupIntervalMin > 60 {
				http.Error(w, "清理间隔必须在1-60分钟之间", http.StatusBadRequest)
				return
			}
			if newConfig.AccessWindowMin <= 0 || newConfig.AccessWindowMin > 1440 {
				http.Error(w, "访问窗口必须在1-1440分钟（24小时）之间", http.StatusBadRequest)
				return
			}
			if newConfig.SyncIntervalSec <= 5 || newConfig.SyncIntervalSec > 300 {
				http.Error(w, "同步间隔必须在5-300秒之间", http.StatusBadRequest)
				return
			}
			if newConfig.CacheValidityMin <= 1 || newConfig.CacheValidityMin > 60 {
				http.Error(w, "缓存有效期必须在1-60分钟之间", http.StatusBadRequest)
				return
			}
			
			// 更新配置
			oldConfig := *cacheConfig
			cacheConfig = &newConfig
			
			// 保存到文件
			if err := saveCacheConfig(); err != nil {
				// 恢复旧配置
				cacheConfig = &oldConfig
				http.Error(w, fmt.Sprintf("保存配置失败: %v", err), http.StatusInternalServerError)
				return
			}
			
			// 重启相关协程以应用新配置
			log.Println("配置已更新，部分功能将在下次启动时完全生效")
			
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
			return
		}
	default:
		http.Error(w, "未知操作", http.StatusBadRequest)
	}
}

// 处理缓存列表请求
func handleCacheList(w http.ResponseWriter, r *http.Request) {
	// 密码验证（仅对 HTML 页面）
	if r.Header.Get("Accept") != "" && strings.Contains(r.Header.Get("Accept"), "text/html") {
		// 检查是否已验证
		cookie, err := r.Cookie("auth")
		if err != nil || cookie.Value != hashPassword(adminPassword) {
			// 显示登录页面
			if r.Method == "POST" {
				// 处理登录请求
				r.ParseForm()
				password := r.FormValue("password")
				if password == adminPassword {
					// 设置 cookie
					http.SetCookie(w, &http.Cookie{
						Name:     "auth",
						Value:    hashPassword(adminPassword),
						Path:     "/",
						MaxAge:   3600, // 1小时
						HttpOnly: true,
					})
					http.Redirect(w, r, "/cache", http.StatusSeeOther)
					return
				} else {
					showLoginPage(w, "密码错误")
					return
				}
			}
			showLoginPage(w, "")
			return
		}
	}
	
	// 解析查询参数
	pageStr := r.URL.Query().Get("page")
	pageSizeStr := r.URL.Query().Get("page_size")
	sortBy := r.URL.Query().Get("sort")
	format := r.URL.Query().Get("format")

	// 设置默认值
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

	// 检查是否请求HTML页面
	if r.Header.Get("Accept") != "" && strings.Contains(r.Header.Get("Accept"), "text/html") {
		// 返回HTML页面
		handleCacheListHTML(w, r, page, pageSize, sortBy)
		return
	}

	// 构建SQL查询
	var whereClause string
	var args []interface{}
	if format != "" {
		whereClause = "WHERE format = ?"
		args = append(args, format)
	}

	// 排序
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

	// 获取总数
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

	// 获取分页数据
	offset := (page - 1) * pageSize
	var query string
	if whereClause != "" {
		query = fmt.Sprintf("SELECT url, file_path, thumb_path, format, access_count, last_access, created_at FROM cache %s %s LIMIT ? OFFSET ?", whereClause, orderBy)
	} else {
		query = fmt.Sprintf("SELECT url, file_path, thumb_path, format, access_count, last_access, created_at FROM cache %s LIMIT ? OFFSET ?", orderBy)
	}
	queryArgs := append(args, pageSize, offset)

	rows, err := queryWithRetry(query, queryArgs...)
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

		// 解析时间 - 支持多种格式
		for _, format := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05"} {
			if item.LastAccess, err = time.Parse(format, lastAccessStr); err == nil {
				break
			}
		}
		if err != nil {
			log.Printf("解析最后访问时间失败: %v", err)
		}
		
		for _, format := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05"} {
			if item.CreatedAt, err = time.Parse(format, createdAtStr); err == nil {
				break
			}
		}
		if err != nil {
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

// handleCacheListHTML 处理缓存列表HTML页面请求
func handleCacheListHTML(w http.ResponseWriter, r *http.Request, page, pageSize int, sortBy string) {
	// 获取语言设置
	lang := getLang(r)
	
	// 生成HTML内容
	html := generateMultiLangHTML(lang, page, pageSize, sortBy)
	
	// 发送响应
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// 生成多语言HTML内容
func generateMultiLangHTML(lang *Language, page, pageSize int, sortBy string) string {
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
            padding: 20px;
            background: linear-gradient(135deg, #f5f3ff 0%, #fef5f5 100%);
            border-bottom: 2px solid #e9ecef;
        }
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-top: 15px;
        }
        .stat-card {
            background: white;
            border-radius: 8px;
            padding: 15px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.05);
            transition: transform 0.2s;
        }
        .stat-card:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 8px rgba(0,0,0,0.1);
        }
        .stat-label {
            font-size: 12px;
            color: #6c757d;
            margin-bottom: 5px;
            display: flex;
            align-items: center;
            gap: 5px;
        }
        .stat-value {
            font-size: 24px;
            font-weight: bold;
            color: #333;
        }
        .stat-unit {
            font-size: 14px;
            color: #6c757d;
            font-weight: normal;
            margin-left: 4px;
        }
        .hit-rate-bar {
            width: 100%;
            height: 20px;
            background: #e9ecef;
            border-radius: 10px;
            overflow: hidden;
            margin-top: 10px;
            position: relative;
        }
        .hit-rate-fill {
            height: 100%;
            background: linear-gradient(90deg, #28a745 0%, #20c997 100%);
            transition: width 0.5s ease;
        }
        .hit-rate-text {
            position: absolute;
            width: 100%;
            text-align: center;
            line-height: 20px;
            font-size: 12px;
            color: #333;
            font-weight: bold;
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
            <div style="position: absolute; top: 20px; right: 20px;">
                <select id="langSelect" onchange="switchLanguage(this.value)" style="background: rgba(255,255,255,0.2); color: white; border: 1px solid white; padding: 5px 10px; border-radius: 4px; cursor: pointer;">
                    <option value="zh" style="color: black;">🇨🇳 中文</option>
                    <option value="en" style="color: black;">🇺🇸 English</option>
                </select>
            </div>
            <h1>🖼️ <span data-i18n="title">缓存图片管理</span></h1>
            <p data-i18n="subtitle">查看和管理所有缓存的图片文件</p>
        </div>
        
        <div class="controls">
            <select id="sortSelect" onchange="updateList()">
                <option value="last_access" data-i18n="sort_last_access">按最后访问时间排序</option>
                <option value="access_count" data-i18n="sort_access_count">按访问次数排序</option>
                <option value="created_at" data-i18n="sort_created_at">按创建时间排序</option>
                <option value="url" data-i18n="sort_url">按URL排序</option>
            </select>
            
            <select id="formatSelect" onchange="updateList()">
                <option value="" data-i18n="format_all">所有格式</option>
                <option value="webp">WebP</option>
                <option value="gif">GIF</option>
                <option value="png">PNG</option>
                <option value="jpeg">JPEG</option>
            </select>
            
            <input type="number" id="pageSizeInput" data-i18n-placeholder="label_page_size" placeholder="每页数量" min="1" max="100" value="20" onchange="updateList()">
            
            <button onclick="refreshList()" data-i18n="btn_refresh">🔄 刷新</button>
            <button onclick="window.open('/stats', '_blank')" data-i18n="btn_stats">📊 统计信息</button>
        </div>
        
        <div class="stats" id="statsContainer">
            <div style="display: flex; justify-content: space-between; align-items: center;">
                <h3 style="margin: 0; color: #333;">📊 实时统计</h3>
                <div style="display: flex; gap: 10px; align-items: center;">
                    <div id="memCacheStatus" style="padding: 4px 8px; border-radius: 4px; font-size: 12px; background: #e8f5e9; color: #2e7d32;">
                        内存缓存: <strong id="memCacheLabel">启用</strong>
                    </div>
                    <button onclick="toggleMemCache()" style="background: #4caf50; color: white; border: none; padding: 6px 12px; border-radius: 4px; cursor: pointer; font-size: 12px;">切换缓存</button>
                    <button onclick="syncToDB()" style="background: #ff9800; color: white; border: none; padding: 6px 12px; border-radius: 4px; cursor: pointer; font-size: 12px;">立即同步</button>
                    <button onclick="showConfigModal()" style="background: #2196f3; color: white; border: none; padding: 6px 12px; border-radius: 4px; cursor: pointer; font-size: 12px;">⚙️ 配置</button>
                    <button onclick="loadStats()" style="background: #6c757d; color: white; border: none; padding: 6px 12px; border-radius: 4px; cursor: pointer; font-size: 12px;">刷新统计</button>
                </div>
            </div>
            <div class="stats-grid" id="statsInfo">
                正在加载统计信息...
            </div>
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
        
        // 设置初始值
        document.getElementById('sortSelect').value = currentSort;
        document.getElementById('pageSizeInput').value = currentPageSize;
        
        function updateList() {
            currentPage = 1; // 重置到第一页
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
                document.getElementById('imageGrid').innerHTML = '<div class="no-data">' + t('msg_loading_failed') + '</div>';
            });
        }
        
        function renderImageGrid(items) {
            const grid = document.getElementById('imageGrid');
            
            if (!items || items.length === 0) {
                grid.innerHTML = '<div class="no-data">' + t('msg_no_cache') + '</div>';
                return;
            }
            
            grid.innerHTML = items.map(item => {
                const thumbUrl = item.thumb_path ? '/thumb/' + item.thumb_path.split('/').pop() : '';
                const lastAccess = new Date(item.last_access).toLocaleString(currentLang === 'zh' ? 'zh-CN' : 'en-US');
                const createdAt = new Date(item.created_at).toLocaleString(currentLang === 'zh' ? 'zh-CN' : 'en-US');
                
                return '<div class="card">' +
                    '<div class="card-image">' +
                    (thumbUrl ? 
                        '<img src="' + thumbUrl + '" alt="' + t('msg_no_thumbnail') + '" onerror="this.style.display=\'none\'; this.nextElementSibling.style.display=\'block\'">' +
                        '<div style="display:none; color:#999; font-size:12px;">' + t('msg_no_thumbnail') + '</div>' :
                        '<div style="color:#999; font-size:12px;">' + t('msg_no_thumbnail') + '</div>'
                    ) +
                    '</div>' +
                    '<div class="card-content">' +
                        '<div class="card-url" title="' + item.url + '">' + item.url + '</div>' +
                        '<div class="card-meta">' +
                            '<div>' +
                                '<span class="format-badge">' + item.format + '</span>' +
                                '<span class="access-count">' + item.access_count + t('label_times_accessed') + '</span>' +
                            '</div>' +
                        '</div>' +
                        '<div style="font-size:11px; color:#aaa; margin-top:8px;">' +
                            '<div>' + t('label_last_access') + ': ' + lastAccess + '</div>' +
                            '<div>' + t('label_created') + ': ' + createdAt + '</div>' +
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
            
            // 上一页
            if (data.page > 1) {
                html += '<a href="#" onclick="goToPage(' + (data.page - 1) + ')">' + t('pagination_prev') + '</a>';
            }
            
            // 页码
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
            
            // 下一页
            if (data.page < data.total_pages) {
                html += '<a href="#" onclick="goToPage(' + (data.page + 1) + ')">' + t('pagination_next') + '</a>';
            }
            
            pagination.innerHTML = html;
        }
        
        function updateStats(data) {
            // 这个函数现在只更新页面信息，统计信息由 loadStats 处理
        }
        
        function formatBytes(bytes) {
            if (bytes === 0) return '0 B';
            const k = 1024;
            const sizes = ['B', 'KB', 'MB', 'GB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
        }
        
        function formatNumber(num) {
            return num.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ",");
        }
        
        function loadStats() {
            fetch('/stats')
                .then(response => response.json())
                .then(data => {
                    const statsInfo = document.getElementById('statsInfo');
                    
                    // 从嵌套的 JSON 结构中提取数据
                    const totalRequests = data.request_stats ? data.request_stats.total_requests : 0;
                    const cacheHits = data.cache_stats ? data.cache_stats.hits : 0;
                    const cacheMisses = data.cache_stats ? data.cache_stats.misses : 0;
                    const hitRate = data.cache_stats ? data.cache_stats.hit_rate : 0;
                    const cacheFiles = data.cache_stats ? data.cache_stats.file_count : 0;
                    const cacheSizeMB = data.cache_stats ? data.cache_stats.size_mb : 0;
                    const spaceSavedMB = data.savings_stats ? data.savings_stats.total_space_saved_mb : 0;
                    const bandwidthSavedMB = data.savings_stats ? data.savings_stats.total_bandwidth_saved_mb : 0;
                    
                    // 转换 MB 到字节
                    const cacheSize = cacheSizeMB * 1024 * 1024;
                    const spaceSaved = spaceSavedMB * 1024 * 1024;
                    const bandwidthSaved = bandwidthSavedMB * 1024 * 1024;
                    
                    statsInfo.innerHTML = 
                        '<div class="stat-card">' +
                            '<div class="stat-label">📥 总请求数</div>' +
                            '<div class="stat-value">' + formatNumber(totalRequests) + '</div>' +
                        '</div>' +
                        
                        '<div class="stat-card">' +
                            '<div class="stat-label">✅ 缓存命中</div>' +
                            '<div class="stat-value">' + formatNumber(cacheHits) + '</div>' +
                        '</div>' +
                        
                        '<div class="stat-card">' +
                            '<div class="stat-label">❌ 缓存未命中</div>' +
                            '<div class="stat-value">' + formatNumber(cacheMisses) + '</div>' +
                        '</div>' +
                        
                        '<div class="stat-card">' +
                            '<div class="stat-label">📊 命中率</div>' +
                            '<div class="stat-value">' + hitRate + '<span class="stat-unit">%</span></div>' +
                            '<div class="hit-rate-bar">' +
                                '<div class="hit-rate-fill" style="width: ' + hitRate + '%"></div>' +
                                '<div class="hit-rate-text">' + hitRate + '%</div>' +
                            '</div>' +
                        '</div>' +
                        
                        '<div class="stat-card">' +
                            '<div class="stat-label">📁 缓存文件数</div>' +
                            '<div class="stat-value">' + formatNumber(cacheFiles) + '</div>' +
                        '</div>' +
                        
                        '<div class="stat-card">' +
                            '<div class="stat-label">💾 缓存大小</div>' +
                            '<div class="stat-value">' + formatBytes(cacheSize) + '</div>' +
                        '</div>' +
                        
                        '<div class="stat-card">' +
                            '<div class="stat-label">🚀 节省空间</div>' +
                            '<div class="stat-value">' + formatBytes(spaceSaved) + '</div>' +
                        '</div>' +
                        
                        '<div class="stat-card">' +
                            '<div class="stat-label">⚡ 节省带宽</div>' +
                            '<div class="stat-value">' + formatBytes(bandwidthSaved) + '</div>' +
                        '</div>';
                })
                .catch(error => {
                    console.error('加载统计信息失败:', error);
                    document.getElementById('statsInfo').innerHTML = 
                        '<div style="text-align: center; color: #dc3545;">加载统计信息失败</div>';
                });
        }
        
        // 切换内存缓存
        function toggleMemCache() {
            fetch('/cache/control?action=toggle', { method: 'POST' })
                .then(response => response.json())
                .then(data => {
                    const label = document.getElementById('memCacheLabel');
                    const statusDiv = document.getElementById('memCacheStatus');
                    label.textContent = data.enabled ? '启用' : '禁用';
                    if (data.enabled) {
                        statusDiv.style.background = '#e8f5e9';
                        statusDiv.style.color = '#2e7d32';
                    } else {
                        statusDiv.style.background = '#ffebee';
                        statusDiv.style.color = '#c62828';
                    }
                    alert('内存缓存已' + (data.enabled ? '启用' : '禁用'));
                })
                .catch(error => {
                    console.error('Error toggling memory cache:', error);
                    alert('切换内存缓存失败');
                });
        }
        
        // 立即同步到数据库
        function syncToDB() {
            fetch('/cache/control?action=sync', { method: 'POST' })
                .then(response => response.json())
                .then(data => {
                    if (data.status === 'synced') {
                        alert('已同步到数据库');
                    }
                })
                .catch(error => {
                    console.error('Error syncing to DB:', error);
                    alert('同步失败');
                });
        }
        
        // 检查内存缓存状态
        function checkMemCacheStatus() {
            fetch('/cache/control?action=status', { method: 'GET' })
                .then(response => response.json())
                .then(data => {
                    const label = document.getElementById('memCacheLabel');
                    const statusDiv = document.getElementById('memCacheStatus');
                    label.textContent = data.enabled ? '启用' : '禁用';
                    if (data.enabled) {
                        statusDiv.style.background = '#e8f5e9';
                        statusDiv.style.color = '#2e7d32';
                    } else {
                        statusDiv.style.background = '#ffebee';
                        statusDiv.style.color = '#c62828';
                    }
                })
                .catch(error => {
                    console.error('Error checking memory cache status:', error);
                });
        }
        
        // i18n 翻译数据
        const i18n = {
            zh: {
                title: '缓存管理',
                subtitle: '查看和管理所有缓存的图片文件',
                btn_refresh: '刷新',
                btn_stats: '统计信息',
                btn_toggle_cache: '切换缓存',
                btn_sync: '立即同步',
                btn_config: '配置',
                btn_refresh_stats: '刷新统计',
                btn_save: '保存配置',
                btn_cancel: '取消',
                btn_delete: '删除',
                btn_login: '登录',
                label_memory_cache: '内存缓存',
                label_enabled: '启用',
                label_disabled: '禁用',
                label_page_size: '每页显示',
                label_sort: '排序',
                label_filter: '筛选格式',
                label_all: '全部',
                stat_total_requests: '总请求数',
                stat_cache_hits: '缓存命中',
                stat_cache_misses: '缓存未命中',
                stat_hit_rate: '命中率',
                stat_cache_files: '缓存文件',
                stat_cache_size: '缓存大小',
                stat_space_saved: '节省空间',
                stat_bandwidth_saved: '节省带宽',
                config_title: '缓存配置',
                config_max_mem_entries: '内存缓存最大条目数',
                config_max_mem_size: '内存缓存最大大小 (MB)',
                config_max_disk_size: '磁盘缓存最大大小 (MB)',
                config_cleanup_interval: '清理间隔 (分钟)',
                config_access_window: '访问时间窗口 (分钟)',
                config_sync_interval: '数据库同步间隔 (秒)',
                config_cache_validity: '缓存有效期 (分钟)',
                config_access_window_hint: '超过此时间未访问的条目优先清理',
                table_preview: '预览',
                table_url: '原始URL',
                table_size: '大小',
                table_format: '格式',
                table_access_count: '访问次数',
                table_last_access: '最后访问',
                table_created: '创建时间',
                table_actions: '操作',
                msg_loading: '正在加载...',
                msg_config_updated: '配置已更新！部分设置将在下次启动时完全生效。',
                msg_config_save_failed: '保存配置失败',
                msg_cache_toggled_on: '内存缓存已启用',
                msg_cache_toggled_off: '内存缓存已禁用',
                msg_synced: '已同步到数据库',
                msg_deleted: '已删除',
                msg_no_data: '暂无数据',
                msg_no_thumbnail: '无缩略图',
                msg_loading_failed: '加载失败，请稍后重试',
                msg_no_cache: '暂无缓存图片',
                label_times_accessed: '次访问',
                label_last_access: '最后访问',
                label_created: '创建时间',
                pagination_prev: '« 上一页',
                pagination_next: '下一页 »',
                sort_last_access: '按最后访问时间排序',
                sort_access_count: '按访问次数排序',
                sort_created_at: '按创建时间排序',
                sort_url: '按URL排序',
                format_all: '所有格式',
                stats_title: '实时统计'
            },
            en: {
                title: 'Cache Management',
                subtitle: 'View and manage all cached image files',
                btn_refresh: 'Refresh',
                btn_stats: 'Statistics',
                btn_toggle_cache: 'Toggle Cache',
                btn_sync: 'Sync Now',
                btn_config: 'Config',
                btn_refresh_stats: 'Refresh Stats',
                btn_save: 'Save Config',
                btn_cancel: 'Cancel',
                btn_delete: 'Delete',
                btn_login: 'Login',
                label_memory_cache: 'Memory Cache',
                label_enabled: 'Enabled',
                label_disabled: 'Disabled',
                label_page_size: 'Per Page',
                label_sort: 'Sort',
                label_filter: 'Filter Format',
                label_all: 'All',
                stat_total_requests: 'Total Requests',
                stat_cache_hits: 'Cache Hits',
                stat_cache_misses: 'Cache Misses',
                stat_hit_rate: 'Hit Rate',
                stat_cache_files: 'Cache Files',
                stat_cache_size: 'Cache Size',
                stat_space_saved: 'Space Saved',
                stat_bandwidth_saved: 'Bandwidth Saved',
                config_title: 'Cache Configuration',
                config_max_mem_entries: 'Max Memory Cache Entries',
                config_max_mem_size: 'Max Memory Cache Size (MB)',
                config_max_disk_size: 'Max Disk Cache Size (MB)',
                config_cleanup_interval: 'Cleanup Interval (min)',
                config_access_window: 'Access Time Window (min)',
                config_sync_interval: 'DB Sync Interval (sec)',
                config_cache_validity: 'Cache Validity (min)',
                config_access_window_hint: 'Entries not accessed within this time will be cleaned first',
                table_preview: 'Preview',
                table_url: 'Original URL',
                table_size: 'Size',
                table_format: 'Format',
                table_access_count: 'Access Count',
                table_last_access: 'Last Access',
                table_created: 'Created',
                table_actions: 'Actions',
                msg_loading: 'Loading...',
                msg_config_updated: 'Configuration updated! Some settings will take full effect on next restart.',
                msg_config_save_failed: 'Failed to save configuration',
                msg_cache_toggled_on: 'Memory cache enabled',
                msg_cache_toggled_off: 'Memory cache disabled',
                msg_synced: 'Synced to database',
                msg_deleted: 'Deleted',
                msg_no_data: 'No data',
                msg_no_thumbnail: 'No thumbnail',
                msg_loading_failed: 'Loading failed, please try again',
                msg_no_cache: 'No cached images',
                label_times_accessed: ' times accessed',
                label_last_access: 'Last access',
                label_created: 'Created',
                pagination_prev: '« Previous',
                pagination_next: 'Next »',
                sort_last_access: 'Sort by Last Access',
                sort_access_count: 'Sort by Access Count',
                sort_created_at: 'Sort by Created Time',
                sort_url: 'Sort by URL',
                format_all: 'All Formats',
                stats_title: 'Live Statistics'
            }
        };
        
        // 当前语言
        let currentLang = getCookie('lang') || 'zh';
        
        // 获取cookie
        function getCookie(name) {
            const value = '; ' + document.cookie;
            const parts = value.split('; ' + name + '=');
            if (parts.length === 2) return parts.pop().split(';').shift();
        }
        
        // 设置cookie
        function setCookie(name, value, days) {
            const date = new Date();
            date.setTime(date.getTime() + (days * 24 * 60 * 60 * 1000));
            document.cookie = name + '=' + value + '; expires=' + date.toUTCString() + '; path=/';
        }
        
        // 切换语言
        function switchLanguage(lang) {
            currentLang = lang;
            setCookie('lang', lang, 30);
            
            // 发送到服务器
            fetch('/cache/control?action=lang', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ lang: lang })
            });
            
            // 更新页面文本
            updatePageTexts();
        }
        
        // 更新页面所有文本
        function updatePageTexts() {
            const texts = i18n[currentLang];
            
            // 更新所有带data-i18n属性的元素
            document.querySelectorAll('[data-i18n]').forEach(elem => {
                const key = elem.getAttribute('data-i18n');
                if (texts[key]) {
                    elem.textContent = texts[key];
                }
            });
            
            // 更新特定元素
            document.getElementById('memCacheLabel').textContent = 
                document.getElementById('memCacheLabel').textContent === '启用' ? 
                texts.label_enabled : texts.label_disabled;
            
            // 更新刷新和统计信息按钮
            const refreshBtn = document.querySelector('button[onclick="refreshList()"]');
            if (refreshBtn) {
                refreshBtn.innerHTML = '🔄 ' + texts.btn_refresh;
            }
            const statsBtn = document.querySelector('button[onclick*="/stats"]');
            if (statsBtn) {
                statsBtn.innerHTML = '📊 ' + texts.btn_stats;
            }
            
            // 更新其他按钮文本
            const buttons = {
                'toggleMemCache': texts.btn_toggle_cache,
                'syncToDB': texts.btn_sync,
                'showConfigModal': texts.btn_config,
                'loadStats': texts.btn_refresh_stats
            };
            
            for (const [funcName, text] of Object.entries(buttons)) {
                const btn = document.querySelector('button[onclick*="' + funcName + '"]');
                if (btn) {
                    // 保留图标
                    const icon = btn.textContent.match(/[⚙️🔄]/);
                    btn.innerHTML = (icon ? icon[0] + ' ' : '') + text;
                }
            }
            
            // 更新下拉选项
            updateSelectOptions();
        }
        
        // 更新下拉选项文本
        function updateSelectOptions() {
            const texts = i18n[currentLang];
            
            // 更新排序选项
            const sortSelect = document.getElementById('sortSelect');
            if (sortSelect) {
                for (let option of sortSelect.options) {
                    const key = option.getAttribute('data-i18n');
                    if (key && texts[key]) {
                        option.text = texts[key];
                    }
                }
            }
            
            // 更新格式筛选选项
            const formatSelect = document.getElementById('formatSelect');
            if (formatSelect) {
                for (let option of formatSelect.options) {
                    const key = option.getAttribute('data-i18n');
                    if (key && texts[key]) {
                        option.text = texts[key];
                    }
                }
            }
            
            // 更新页面大小输入框占位符
            const pageSizeInput = document.getElementById('pageSizeInput');
            if (pageSizeInput) {
                const key = pageSizeInput.getAttribute('data-i18n-placeholder');
                if (key && texts[key]) {
                    pageSizeInput.placeholder = texts[key];
                }
            }
        }
        
        // 获取翻译文本
        function t(key) {
            return i18n[currentLang][key] || key;
        }
        
        // 页面加载时获取数据
        document.addEventListener('DOMContentLoaded', function() {
            // 设置语言选择器的值
            document.getElementById('langSelect').value = currentLang;
            
            // 更新页面文本
            updatePageTexts();
            
            loadCacheList();
            loadStats();
            checkMemCacheStatus();
            
            // 每30秒自动刷新统计
            setInterval(loadStats, 30000);
        });
    </script>
    
    <!-- 配置模态框 -->
    <div id="configModal" style="display: none; position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0,0,0,0.5); z-index: 1000;">
        <div style="position: absolute; top: 50%; left: 50%; transform: translate(-50%, -50%); background: white; padding: 30px; border-radius: 8px; width: 500px; max-height: 80vh; overflow-y: auto;">
            <h2 style="margin-top: 0;">⚙️ 缓存配置</h2>
            
            <form id="configForm">
                <div style="margin-bottom: 15px;">
                    <label style="display: block; margin-bottom: 5px; font-weight: bold;">内存缓存最大条目数:</label>
                    <input type="number" id="maxMemCacheEntries" min="1" max="10000" style="width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px;">
                </div>
                
                <div style="margin-bottom: 15px;">
                    <label style="display: block; margin-bottom: 5px; font-weight: bold;">内存缓存最大大小 (MB):</label>
                    <input type="number" id="maxMemCacheSizeMB" min="1" max="1000" style="width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px;">
                </div>
                
                <div style="margin-bottom: 15px;">
                    <label style="display: block; margin-bottom: 5px; font-weight: bold;">磁盘缓存最大大小 (MB):</label>
                    <input type="number" id="maxDiskCacheSizeMB" min="1" max="10000" style="width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px;">
                </div>
                
                <div style="margin-bottom: 15px;">
                    <label style="display: block; margin-bottom: 5px; font-weight: bold;">清理间隔 (分钟):</label>
                    <input type="number" id="cleanupIntervalMin" min="1" max="60" style="width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px;">
                </div>
                
                <div style="margin-bottom: 15px;">
                    <label style="display: block; margin-bottom: 5px; font-weight: bold;">访问时间窗口 (分钟):</label>
                    <input type="number" id="accessWindowMin" min="1" max="1440" style="width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px;">
                    <small style="color: #666;">超过此时间未访问的条目优先清理</small>
                </div>
                
                <div style="margin-bottom: 15px;">
                    <label style="display: block; margin-bottom: 5px; font-weight: bold;">数据库同步间隔 (秒):</label>
                    <input type="number" id="syncIntervalSec" min="5" max="300" style="width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px;">
                </div>
                
                <div style="margin-bottom: 20px;">
                    <label style="display: block; margin-bottom: 5px; font-weight: bold;">缓存有效期 (分钟):</label>
                    <input type="number" id="cacheValidityMin" min="1" max="60" style="width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px;">
                </div>
                
                <div style="display: flex; gap: 10px; justify-content: flex-end;">
                    <button type="button" onclick="hideConfigModal()" style="padding: 10px 20px; background: #666; color: white; border: none; border-radius: 4px; cursor: pointer;">取消</button>
                    <button type="submit" style="padding: 10px 20px; background: #2196f3; color: white; border: none; border-radius: 4px; cursor: pointer;">保存配置</button>
                </div>
            </form>
        </div>
    </div>
    
    <script>
        let currentConfig = {};
        
        function showConfigModal() {
            // 加载当前配置
            fetch('/cache/control?action=config', { method: 'GET' })
                .then(response => response.json())
                .then(config => {
                    currentConfig = config;
                    document.getElementById('maxMemCacheEntries').value = config.max_mem_cache_entries;
                    document.getElementById('maxMemCacheSizeMB').value = config.max_mem_cache_size_mb;
                    document.getElementById('maxDiskCacheSizeMB').value = config.max_disk_cache_size_mb;
                    document.getElementById('cleanupIntervalMin').value = config.cleanup_interval_min;
                    document.getElementById('accessWindowMin').value = config.access_window_min;
                    document.getElementById('syncIntervalSec').value = config.sync_interval_sec;
                    document.getElementById('cacheValidityMin').value = config.cache_validity_min;
                    document.getElementById('configModal').style.display = 'block';
                })
                .catch(error => {
                    console.error('加载配置失败:', error);
                    alert('加载配置失败');
                });
        }
        
        function hideConfigModal() {
            document.getElementById('configModal').style.display = 'none';
        }
        
        // 保存配置
        document.getElementById('configForm').addEventListener('submit', function(e) {
            e.preventDefault();
            
            const newConfig = {
                max_mem_cache_entries: parseInt(document.getElementById('maxMemCacheEntries').value),
                max_mem_cache_size_mb: parseInt(document.getElementById('maxMemCacheSizeMB').value),
                max_disk_cache_size_mb: parseInt(document.getElementById('maxDiskCacheSizeMB').value),
                cleanup_interval_min: parseInt(document.getElementById('cleanupIntervalMin').value),
                access_window_min: parseInt(document.getElementById('accessWindowMin').value),
                sync_interval_sec: parseInt(document.getElementById('syncIntervalSec').value),
                cache_validity_min: parseInt(document.getElementById('cacheValidityMin').value)
            };
            
            fetch('/cache/control?action=config', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(newConfig)
            })
            .then(response => {
                if (!response.ok) {
                    return response.text().then(text => { throw new Error(text); });
                }
                return response.json();
            })
            .then(data => {
                if (data.status === 'updated') {
                    alert('配置已更新！部分设置将在下次启动时完全生效。');
                    hideConfigModal();
                    loadStats(); // 刷新统计信息
                }
            })
            .catch(error => {
                console.error('保存配置失败:', error);
                alert('保存配置失败: ' + error.message);
            });
        });
    </script>
</body>
</html>
`

	// 使用Go模板替换变量
	htmlTemplate = strings.ReplaceAll(htmlTemplate, "{{.Page}}", strconv.Itoa(page))
	htmlTemplate = strings.ReplaceAll(htmlTemplate, "{{.PageSize}}", strconv.Itoa(pageSize))
	htmlTemplate = strings.ReplaceAll(htmlTemplate, "{{.Sort}}", sortBy)
	
	return htmlTemplate
}

// handleHealth 健康检查端点
func handleHealth(w http.ResponseWriter, r *http.Request) {
	// 检查数据库连接
	dbStatus := "ok"
	if err := db.Ping(); err != nil {
		dbStatus = "error: " + err.Error()
	}
	
	// 检查缓存目录
	cacheStatus := "ok"
	if _, err := os.Stat(cacheDir); err != nil {
		cacheStatus = "error: " + err.Error()
	}
	
	// 获取内存使用情况
	memCacheCount := lruCache.Len()
	
	// 构建健康状态
	health := map[string]interface{}{
		"status": "healthy",
		"timestamp": time.Now().Unix(),
		"uptime": time.Since(startTime).Seconds(),
		"checks": map[string]interface{}{
			"database": dbStatus,
			"cache_dir": cacheStatus,
		},
		"metrics": map[string]interface{}{
			"total_requests": atomic.LoadInt64(&requestCount),
			"cache_hits": atomic.LoadInt64(&cacheHits),
			"cache_misses": atomic.LoadInt64(&cacheMisses),
			"memory_cache_entries": memCacheCount,
		},
	}
	
	// 如果有任何错误，设置状态为不健康
	if dbStatus != "ok" || cacheStatus != "ok" {
		health["status"] = "unhealthy"
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

// setupGracefulShutdown 设置优雅关闭
func setupGracefulShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	
	go func() {
		<-sigChan
		log.Println("收到关闭信号，开始优雅关闭...")
		
		// 创建超时上下文
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		
		// 停止接受新请求并等待现有请求完成
		if httpServer != nil {
			if err := httpServer.Shutdown(ctx); err != nil {
				log.Printf("HTTP服务器关闭失败: %v", err)
			}
		}
		
		// 停止后台任务
		close(shutdownChan)
		close(cleanupStopChan)
		close(syncStopChan)
		
		// 同步内存缓存到数据库
		if useMemCache {
			log.Println("正在同步内存缓存到数据库...")
			syncToDB()
		}
		
		// 关闭 io 后端进程
		if ioProcess != nil {
			log.Println("正在关闭 io 存储后端...")
			if err := ioProcess.Process.Signal(syscall.SIGTERM); err != nil {
				log.Printf("发送终止信号失败: %v", err)
				ioProcess.Process.Kill()
			}
			ioProcess.Wait()
		}
		
		// 关闭数据库连接
		if db != nil {
			db.Close()
		}
		
		// 关闭日志文件
		closeLogger()
		
		log.Println("优雅关闭完成")
		os.Exit(0)
	}()
}

// generateETag 生成ETag
func generateETag(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf(`"%x"`, hash)
}

// 实现 MemoryStorage
func NewMemoryStorage(maxSize int64) *MemoryStorage {
	return &MemoryStorage{
		cache:   NewLRUCache(1000, int(maxSize/1024/1024)),
		data:    make(map[string][]byte),
		maxSize: maxSize,
	}
}

func (m *MemoryStorage) Store(data []byte, metadata map[string]string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// 检查是否有自定义ID
	id := ""
	if customID, ok := metadata["custom_id"]; ok && customID != "" {
		id = customID
	} else {
		// 生成ID (使用SHA1)
		hasher := sha1.New()
		hasher.Write(data)
		id = hex.EncodeToString(hasher.Sum(nil))
	}
	
	// 检查大小限制
	if int64(len(data)) > m.maxSize {
		return "", fmt.Errorf("文件大小超过内存限制")
	}
	
	// 如果需要释放空间
	for m.currSize+int64(len(data)) > m.maxSize && len(m.data) > 0 {
		// 移除最旧的项（简化实现）
		for oldID := range m.data {
			delete(m.data, oldID)
			m.currSize -= int64(len(m.data[oldID]))
			break
		}
	}
	
	m.data[id] = data
	m.currSize += int64(len(data))
	
	return id, nil
}

func (m *MemoryStorage) Get(id string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	data, exists := m.data[id]
	if !exists {
		return nil, fmt.Errorf("文件不存在: %s", id)
	}
	
	return data, nil
}

func (m *MemoryStorage) Exists(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	_, exists := m.data[id]
	return exists
}

func (m *MemoryStorage) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if data, exists := m.data[id]; exists {
		m.currSize -= int64(len(data))
		delete(m.data, id)
	}
	
	return nil
}

func (m *MemoryStorage) Name() string {
	return "Memory"
}

// 实现 LocalStorage
func NewLocalStorage(basePath string) *LocalStorage {
	os.MkdirAll(basePath, 0755)
	return &LocalStorage{
		basePath: basePath,
	}
}

func (l *LocalStorage) Store(data []byte, metadata map[string]string) (string, error) {
	// 检查是否有自定义ID
	id := ""
	if customID, ok := metadata["custom_id"]; ok && customID != "" {
		id = customID
	} else {
		// 生成ID
		hasher := sha1.New()
		hasher.Write(data)
		id = hex.EncodeToString(hasher.Sum(nil))
	}
	
	// 构建文件路径 (使用前两个字符作为子目录)
	subDir := id[:2]
	dirPath := filepath.Join(l.basePath, subDir)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", err
	}
	
	filePath := filepath.Join(dirPath, id)
	
	// 如果文件已存在，直接返回
	if _, err := os.Stat(filePath); err == nil {
		return id, nil
	}
	
	// 写入文件
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", err
	}
	
	return id, nil
}

func (l *LocalStorage) Get(id string) ([]byte, error) {
	subDir := id[:2]
	filePath := filepath.Join(l.basePath, subDir, id)
	
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %v", err)
	}
	
	return data, nil
}

func (l *LocalStorage) Exists(id string) bool {
	subDir := id[:2]
	filePath := filepath.Join(l.basePath, subDir, id)
	
	_, err := os.Stat(filePath)
	return err == nil
}

func (l *LocalStorage) Delete(id string) error {
	subDir := id[:2]
	filePath := filepath.Join(l.basePath, subDir, id)
	
	return os.Remove(filePath)
}

func (l *LocalStorage) Name() string {
	return "Local"
}

// 实现 IOBackendStorage
func NewIOBackendStorage(apiURL, apiKey string) *IOBackendStorage {
	return &IOBackendStorage{
		apiURL:  apiURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 30 * time.Second},
		enabled: true,
	}
}

func (i *IOBackendStorage) Store(data []byte, metadata map[string]string) (string, error) {
	if !i.enabled {
		return "", fmt.Errorf("io后端未启用")
	}
	
	// 检查是否有自定义ID
	sha1Hash := ""
	if customID, ok := metadata["custom_id"]; ok && customID != "" {
		sha1Hash = customID
	} else {
		// 计算SHA1
		hasher := sha1.New()
		hasher.Write(data)
		sha1Hash = hex.EncodeToString(hasher.Sum(nil))
	}
	
	// 检查是否已存在
	if i.Exists(sha1Hash) {
		return sha1Hash, nil
	}
	
	// 上传文件
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "data")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	writer.Close()
	
	req, err := http.NewRequest("POST", i.apiURL+"/api/store", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-API-Key", i.apiKey)
	
	resp, err := i.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("上传失败: HTTP %d", resp.StatusCode)
	}
	
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	
	if id, ok := result["sha1"].(string); ok {
		return id, nil
	}
	
	return sha1Hash, nil
}

func (i *IOBackendStorage) Get(id string) ([]byte, error) {
	if !i.enabled {
		return nil, fmt.Errorf("io后端未启用")
	}
	
	req, err := http.NewRequest("GET", i.apiURL+"/api/file/"+id, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", i.apiKey)
	
	resp, err := i.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取文件失败: HTTP %d", resp.StatusCode)
	}
	
	return io.ReadAll(resp.Body)
}

func (i *IOBackendStorage) Exists(id string) bool {
	if !i.enabled {
		return false
	}
	
	req, err := http.NewRequest("GET", i.apiURL+"/api/exists/"+id, nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-API-Key", i.apiKey)
	
	resp, err := i.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	
	return resp.StatusCode == http.StatusOK
}

func (i *IOBackendStorage) Delete(id string) error {
	if !i.enabled {
		return fmt.Errorf("io后端未启用")
	}
	
	req, err := http.NewRequest("DELETE", i.apiURL+"/api/file/"+id, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", i.apiKey)
	
	resp, err := i.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("删除失败: HTTP %d", resp.StatusCode)
	}
	
	return nil
}

func (i *IOBackendStorage) Name() string {
	return "IOBackend"
}

// 实现 StorageManager
func NewStorageManager(config StorageConfig) *StorageManager {
	sm := &StorageManager{
		backends: make([]StorageBackend, 0),
	}
	
	// 按优先级添加存储后端：内存 -> 本地 -> 远程
	if config.EnableMemory {
		sm.backends = append(sm.backends, NewMemoryStorage(config.MemoryMaxSize))
		log.Println("启用内存存储层")
	}
	
	if config.EnableLocal {
		sm.backends = append(sm.backends, NewLocalStorage(config.LocalPath))
		log.Println("启用本地存储层")
	}
	
	if config.EnableRemote && config.RemoteAPIKey != "" {
		sm.backends = append(sm.backends, NewIOBackendStorage(config.RemoteURL, config.RemoteAPIKey))
		log.Println("启用远程io存储层")
	}
	
	return sm
}

// Store 分层存储文件
func (sm *StorageManager) Store(data []byte, metadata map[string]string) (string, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	if len(sm.backends) == 0 {
		return "", fmt.Errorf("没有可用的存储后端")
	}
	
	var lastErr error
	var fileID string
	
	// 尝试存储到最后一层（通常是最持久的）
	for i := len(sm.backends) - 1; i >= 0; i-- {
		backend := sm.backends[i]
		id, err := backend.Store(data, metadata)
		if err == nil {
			fileID = id
			log.Printf("文件存储到 %s: %s", backend.Name(), id)
			
			// 成功存储后，向上层缓存（异步）
			go func(upperBackends []StorageBackend, data []byte, id string) {
				for j := i - 1; j >= 0; j-- {
					if _, err := upperBackends[j].Store(data, metadata); err == nil {
						log.Printf("文件缓存到 %s: %s", upperBackends[j].Name(), id)
					}
				}
			}(sm.backends, data, id)
			
			return fileID, nil
		}
		lastErr = err
		log.Printf("存储到 %s 失败: %v", backend.Name(), err)
	}
	
	return "", fmt.Errorf("所有存储后端都失败: %v", lastErr)
}

// StorageResult 存储结果，包含数据和层级信息
type StorageResult struct {
	Data      []byte
	CacheLevel string
}

// Get 分层获取文件
func (sm *StorageManager) Get(id string) ([]byte, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	var lastErr error
	
	// 从最快的层开始查找
	for i, backend := range sm.backends {
		data, err := backend.Get(id)
		if err == nil {
			atomic.AddInt64(&cacheHits, 1)
			log.Printf("从 %s 获取文件: %s", backend.Name(), id)
			
			// 如果不是从第一层获取的，缓存到上层（异步）
			if i > 0 {
				go func(upperBackends []StorageBackend, data []byte, id string) {
					for j := i - 1; j >= 0; j-- {
						if _, err := upperBackends[j].Store(data, nil); err == nil {
							log.Printf("文件缓存到 %s: %s", upperBackends[j].Name(), id)
						}
					}
				}(sm.backends, data, id)
			}
			
			return data, nil
		}
		lastErr = err
	}
	
	atomic.AddInt64(&cacheMisses, 1)
	return nil, fmt.Errorf("文件未找到: %v", lastErr)
}

// GetWithLevel 分层获取文件，返回缓存层级信息
func (sm *StorageManager) GetWithLevel(id string) (*StorageResult, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	var lastErr error
	
	// 从最快的层开始查找
	for i, backend := range sm.backends {
		data, err := backend.Get(id)
		if err == nil {
			atomic.AddInt64(&cacheHits, 1)
			cacheLevel := backend.Name()
			log.Printf("从 %s 获取文件: %s", cacheLevel, id)
			
			// 如果不是从第一层获取的，缓存到上层（异步）
			if i > 0 {
				go func(upperBackends []StorageBackend, data []byte, id string) {
					for j := i - 1; j >= 0; j-- {
						if _, err := upperBackends[j].Store(data, nil); err == nil {
							log.Printf("文件缓存到 %s: %s", upperBackends[j].Name(), id)
						}
					}
				}(sm.backends, data, id)
			}
			
			return &StorageResult{
				Data:       data,
				CacheLevel: cacheLevel,
			}, nil
		}
		lastErr = err
	}
	
	atomic.AddInt64(&cacheMisses, 1)
	return nil, fmt.Errorf("文件未找到: %v", lastErr)
}

// Exists 检查文件是否存在
func (sm *StorageManager) Exists(id string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	for _, backend := range sm.backends {
		if backend.Exists(id) {
			return true
		}
	}
	
	return false
}

// Delete 从所有层删除文件
func (sm *StorageManager) Delete(id string) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	var lastErr error
	deleted := false
	
	// 从所有层删除
	for _, backend := range sm.backends {
		if err := backend.Delete(id); err == nil {
			deleted = true
			log.Printf("从 %s 删除文件: %s", backend.Name(), id)
		} else {
			lastErr = err
		}
	}
	
	if deleted {
		return nil
	}
	
	return lastErr
}

// NewLRUCache 创建新的LRU缓存
func NewLRUCache(maxEntries int, maxSizeMB int) *LRUCache {
	return &LRUCache{
		entries:    make(map[string]*CacheEntry),
		maxEntries: maxEntries,
		maxSizeMB:  maxSizeMB,
	}
}

// handleUpload 处理上传页面
func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// 获取用户语言偏好
	langObj := getLang(r)
	lang := langObj.Code
	
	// 构建页面HTML
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="%s">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            min-height: 100vh;
            display: flex;
            justify-content: center;
            align-items: center;
            padding: 20px;
        }
        .container {
            background: rgba(255, 255, 255, 0.95);
            border-radius: 20px;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
            max-width: 600px;
            width: 100%%;
            padding: 40px;
        }
        h1 {
            color: #333;
            margin-bottom: 30px;
            text-align: center;
            font-size: 2rem;
        }
        .upload-area {
            border: 3px dashed #667eea;
            border-radius: 15px;
            padding: 40px;
            text-align: center;
            cursor: pointer;
            transition: all 0.3s;
            background: #f9f9ff;
        }
        .upload-area:hover, .upload-area.dragover {
            background: #f0f0ff;
            border-color: #764ba2;
        }
        .upload-icon {
            font-size: 48px;
            color: #667eea;
            margin-bottom: 20px;
        }
        input[type="file"] {
            display: none;
        }
        .upload-text {
            color: #666;
            font-size: 16px;
            margin-bottom: 10px;
        }
        .upload-subtext {
            color: #999;
            font-size: 14px;
        }
        .upload-button {
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            color: white;
            border: none;
            padding: 12px 30px;
            border-radius: 25px;
            font-size: 16px;
            cursor: pointer;
            margin-top: 20px;
            transition: transform 0.2s;
            display: none;
        }
        .upload-button:hover {
            transform: scale(1.05);
        }
        .upload-button:active {
            transform: scale(0.95);
        }
        .preview-container {
            margin-top: 30px;
            display: none;
        }
        .preview-image {
            max-width: 100%%;
            border-radius: 10px;
            box-shadow: 0 5px 15px rgba(0, 0, 0, 0.2);
        }
        .file-info {
            margin-top: 15px;
            padding: 15px;
            background: #f5f5f5;
            border-radius: 10px;
            font-size: 14px;
            color: #666;
        }
        .progress-bar {
            width: 100%%;
            height: 6px;
            background: #e0e0e0;
            border-radius: 3px;
            overflow: hidden;
            margin-top: 20px;
            display: none;
        }
        .progress-fill {
            height: 100%%;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            width: 0%%;
            transition: width 0.3s;
        }
        .result {
            margin-top: 30px;
            padding: 20px;
            background: #f0fff4;
            border: 1px solid #4caf50;
            border-radius: 10px;
            display: none;
        }
        .result-title {
            color: #4caf50;
            font-weight: bold;
            margin-bottom: 10px;
        }
        .result-link {
            color: #667eea;
            word-break: break-all;
            text-decoration: none;
        }
        .result-link:hover {
            text-decoration: underline;
        }
        .error {
            background: #fff0f0;
            border-color: #f44336;
        }
        .error .result-title {
            color: #f44336;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>%s</h1>
        <div class="upload-area" id="uploadArea">
            <div class="upload-icon">📁</div>
            <div class="upload-text">%s</div>
            <div class="upload-subtext">%s</div>
            <input type="file" id="fileInput" accept="image/*" multiple>
        </div>
        <button class="upload-button" id="uploadButton">%s</button>
        <div class="progress-bar" id="progressBar">
            <div class="progress-fill" id="progressFill"></div>
        </div>
        <div class="preview-container" id="previewContainer">
            <img class="preview-image" id="previewImage" alt="Preview">
            <div class="file-info" id="fileInfo"></div>
        </div>
        <div class="result" id="result">
            <div class="result-title" id="resultTitle"></div>
            <div id="resultContent"></div>
        </div>
    </div>
    
    <script>
        const uploadArea = document.getElementById('uploadArea');
        const fileInput = document.getElementById('fileInput');
        const uploadButton = document.getElementById('uploadButton');
        const previewContainer = document.getElementById('previewContainer');
        const previewImage = document.getElementById('previewImage');
        const fileInfo = document.getElementById('fileInfo');
        const progressBar = document.getElementById('progressBar');
        const progressFill = document.getElementById('progressFill');
        const result = document.getElementById('result');
        const resultTitle = document.getElementById('resultTitle');
        const resultContent = document.getElementById('resultContent');
        
        let selectedFiles = [];
        
        // 点击上传区域
        uploadArea.addEventListener('click', () => {
            fileInput.click();
        });
        
        // 拖拽事件
        uploadArea.addEventListener('dragover', (e) => {
            e.preventDefault();
            uploadArea.classList.add('dragover');
        });
        
        uploadArea.addEventListener('dragleave', () => {
            uploadArea.classList.remove('dragover');
        });
        
        uploadArea.addEventListener('drop', (e) => {
            e.preventDefault();
            uploadArea.classList.remove('dragover');
            handleFiles(e.dataTransfer.files);
        });
        
        // 文件选择
        fileInput.addEventListener('change', (e) => {
            handleFiles(e.target.files);
        });
        
        // 处理文件
        function handleFiles(files) {
            selectedFiles = Array.from(files).filter(file => file.type.startsWith('image/'));
            
            if (selectedFiles.length === 0) {
                alert('%s');
                return;
            }
            
            // 显示预览
            const file = selectedFiles[0];
            const reader = new FileReader();
            reader.onload = (e) => {
                previewImage.src = e.target.result;
                previewContainer.style.display = 'block';
                fileInfo.innerHTML = '%s' + file.name + '<br>%s' + formatFileSize(file.size) + '<br>%s' + file.type;
            };
            reader.readAsDataURL(file);
            
            uploadButton.style.display = 'inline-block';
            result.style.display = 'none';
        }
        
        // 格式化文件大小
        function formatFileSize(bytes) {
            if (bytes === 0) return '0 Bytes';
            const k = 1024;
            const sizes = ['Bytes', 'KB', 'MB', 'GB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
        }
        
        // 上传按钮点击
        uploadButton.addEventListener('click', async () => {
            if (selectedFiles.length === 0) return;
            
            uploadButton.disabled = true;
            progressBar.style.display = 'block';
            result.style.display = 'none';
            
            const formData = new FormData();
            selectedFiles.forEach(file => {
                formData.append('images', file);
            });
            
            try {
                const xhr = new XMLHttpRequest();
                
                xhr.upload.addEventListener('progress', (e) => {
                    if (e.lengthComputable) {
                        const percentComplete = (e.loaded / e.total) * 100;
                        progressFill.style.width = percentComplete + '%%';
                    }
                });
                
                xhr.addEventListener('load', () => {
                    if (xhr.status === 200) {
                        const response = JSON.parse(xhr.responseText);
                        showResult(response);
                    } else {
                        showError('%s');
                    }
                    uploadButton.disabled = false;
                    progressBar.style.display = 'none';
                    progressFill.style.width = '0%%';
                });
                
                xhr.addEventListener('error', () => {
                    showError('%s');
                    uploadButton.disabled = false;
                    progressBar.style.display = 'none';
                    progressFill.style.width = '0%%';
                });
                
                xhr.open('POST', '/api/upload');
                xhr.send(formData);
                
            } catch (error) {
                showError('%s' + error.message);
                uploadButton.disabled = false;
                progressBar.style.display = 'none';
                progressFill.style.width = '0%%';
            }
        });
        
        // 显示结果
        function showResult(response) {
            result.className = 'result';
            result.style.display = 'block';
            resultTitle.textContent = '%s';
            
            let html = '';
            response.urls.forEach(url => {
                const fullUrl = window.location.origin + url;
                html += '<div style="margin-bottom: 15px;">';
                html += '<div>%s</div>';
                html += '<a href="' + fullUrl + '" target="_blank" class="result-link">' + fullUrl + '</a>';
                html += '<div style="margin-top: 5px;">';
                html += '<button onclick="copyToClipboard(\'' + fullUrl + '\')" style="margin-right: 10px; padding: 5px 10px; border: 1px solid #667eea; background: white; color: #667eea; border-radius: 5px; cursor: pointer;">%s</button>';
                html += '<button onclick="copyToClipboard(\'' + fullUrl + '?format=webp\')" style="padding: 5px 10px; border: 1px solid #667eea; background: white; color: #667eea; border-radius: 5px; cursor: pointer;">%s</button>';
                html += '</div>';
                html += '</div>';
            });
            resultContent.innerHTML = html;
        }
        
        // 显示错误
        function showError(message) {
            result.className = 'result error';
            result.style.display = 'block';
            resultTitle.textContent = '%s';
            resultContent.textContent = message;
        }
        
        // 复制到剪贴板
        function copyToClipboard(text) {
            navigator.clipboard.writeText(text).then(() => {
                alert('%s');
            }).catch(() => {
                alert('%s');
            });
        }
    </script>
</body>
</html>`,
		lang,
		map[bool]string{true: "图片上传", false: "Image Upload"}[lang == "zh"],
		map[bool]string{true: "图片上传", false: "Image Upload"}[lang == "zh"],
		map[bool]string{true: "点击或拖拽图片到这里", false: "Click or drag images here"}[lang == "zh"],
		map[bool]string{true: "支持 JPG, PNG, GIF, WebP 等格式", false: "Supports JPG, PNG, GIF, WebP formats"}[lang == "zh"],
		map[bool]string{true: "上传图片", false: "Upload Images"}[lang == "zh"],
		map[bool]string{true: "请选择图片文件", false: "Please select image files"}[lang == "zh"],
		map[bool]string{true: "文件名: ", false: "Filename: "}[lang == "zh"],
		map[bool]string{true: "大小: ", false: "Size: "}[lang == "zh"],
		map[bool]string{true: "类型: ", false: "Type: "}[lang == "zh"],
		map[bool]string{true: "上传失败", false: "Upload failed"}[lang == "zh"],
		map[bool]string{true: "网络错误", false: "Network error"}[lang == "zh"],
		map[bool]string{true: "上传错误: ", false: "Upload error: "}[lang == "zh"],
		map[bool]string{true: "上传成功！", false: "Upload successful!"}[lang == "zh"],
		map[bool]string{true: "图片链接：", false: "Image URL:"}[lang == "zh"],
		map[bool]string{true: "复制链接", false: "Copy URL"}[lang == "zh"],
		map[bool]string{true: "复制WebP链接", false: "Copy WebP URL"}[lang == "zh"],
		map[bool]string{true: "错误", false: "Error"}[lang == "zh"],
		map[bool]string{true: "已复制到剪贴板", false: "Copied to clipboard"}[lang == "zh"],
		map[bool]string{true: "复制失败", false: "Copy failed"}[lang == "zh"],
	)
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, html)
}

// storeToIOBackend 将文件存储到 io 后端
func storeToIOBackend(data []byte) (string, error) {
	// 计算 SHA1
	hasher := sha1.New()
	hasher.Write(data)
	sha1Hash := hex.EncodeToString(hasher.Sum(nil))
	
	// 检查文件是否已存在
	checkURL := fmt.Sprintf("%s/api/exists/%s", ioBackendURL, sha1Hash)
	req, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-API-Key", ioAPIKey)
	
	resp, err := http.DefaultClient.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		resp.Body.Close()
		// 文件已存在
		return sha1Hash, nil
	}
	if resp != nil {
		resp.Body.Close()
	}
	
	// 上传文件到 io 后端
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "image")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	writer.Close()
	
	uploadURL := fmt.Sprintf("%s/api/store", ioBackendURL)
	req, err = http.NewRequest("POST", uploadURL, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-API-Key", ioAPIKey)
	
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("上传失败: HTTP %d", resp.StatusCode)
	}
	
	// 解析响应
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	
	if sha1Str, ok := result["sha1"].(string); ok {
		return sha1Str, nil
	}
	
	return sha1Hash, nil
}

// getFromIOBackend 从 io 后端获取文件
func getFromIOBackend(sha1Hash string) ([]byte, error) {
	url := fmt.Sprintf("%s/api/file/%s", ioBackendURL, sha1Hash)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", ioAPIKey)
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取文件失败: HTTP %d", resp.StatusCode)
	}
	
	return io.ReadAll(resp.Body)
}

// handleAPIUpload 处理图片上传API
func handleAPIUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// 解析multipart form，限制32MB
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}
	
	files := r.MultipartForm.File["images"]
	if len(files) == 0 {
		http.Error(w, "No files uploaded", http.StatusBadRequest)
		return
	}
	
	var uploadedURLs []string
	
	for _, fileHeader := range files {
		// 打开上传的文件
		file, err := fileHeader.Open()
		if err != nil {
			log.Printf("打开上传文件失败: %v", err)
			continue
		}
		defer file.Close()
		
		// 读取文件内容
		data, err := io.ReadAll(file)
		if err != nil {
			log.Printf("读取上传文件失败: %v", err)
			continue
		}
		
		// 检测图片格式
		contentType := http.DetectContentType(data)
		if !strings.HasPrefix(contentType, "image/") {
			log.Printf("不支持的文件类型: %s", contentType)
			continue
		}
		
		// 准备元数据
		metadata := map[string]string{
			"filename":     fileHeader.Filename,
			"content_type": contentType,
			"size":         strconv.Itoa(len(data)),
		}
		
		// 使用存储管理器存储文件
		fileID, err := storageManager.Store(data, metadata)
		if err != nil {
			log.Printf("存储文件失败: %v", err)
			continue
		}
		
		// 获取文件扩展名
		ext := filepath.Ext(fileHeader.Filename)
		if ext == "" {
			switch contentType {
			case "image/jpeg":
				ext = ".jpg"
			case "image/png":
				ext = ".png"
			case "image/gif":
				ext = ".gif"
			case "image/webp":
				ext = ".webp"
			default:
				ext = ".jpg"
			}
		}
		
		// 保存元数据到数据库
		fileURL := "/storage/" + fileID + ext
		_, err = db.Exec(`
			INSERT OR REPLACE INTO cache (url, file_path, created_at, file_size, content_type, width, height)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, fileURL, fileID, time.Now().Unix(), len(data), contentType, 0, 0)
		if err != nil {
			log.Printf("保存元数据失败: %v", err)
		}
		
		uploadedURLs = append(uploadedURLs, fileURL)
	}
	
	if len(uploadedURLs) == 0 {
		http.Error(w, "No images uploaded successfully", http.StatusInternalServerError)
		return
	}
	
	// 返回JSON响应
	response := map[string]interface{}{
		"success": true,
		"urls":    uploadedURLs,
		"count":   len(uploadedURLs),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleStorageFiles 处理从存储管理器获取文件的请求
func handleStorageFiles(w http.ResponseWriter, r *http.Request) {
	// 获取文件路径
	path := strings.TrimPrefix(r.URL.Path, "/storage/")
	if path == "" {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	
	// 提取文件ID（去掉扩展名）
	fileID := path
	if idx := strings.LastIndex(path, "."); idx > 0 {
		fileID = path[:idx]
	}
	
	// 获取查询参数
	query := r.URL.Query()
	format := query.Get("format")
	widthStr := query.Get("w")
	heightStr := query.Get("h")
	mode := query.Get("mode")
	qualityStr := query.Get("q")
	
	// 生成变换缓存键（用于缓存变换后的图片）
	transformKey := fileID
	if format != "" || widthStr != "" || heightStr != "" || qualityStr != "" {
		transformKey = fmt.Sprintf("%s_f%s_w%s_h%s_m%s_q%s", 
			fileID, format, widthStr, heightStr, mode, qualityStr)
	}
	
	// 先尝试从缓存获取变换后的图片
	var result *StorageResult
	var err error
	var isTransformed bool
	
	if transformKey != fileID {
		// 有变换参数，先尝试获取变换后的缓存
		result, err = storageManager.GetWithLevel(transformKey)
		if err == nil {
			isTransformed = true
			log.Printf("获取变换后的缓存: %s", transformKey)
		}
	}
	
	// 如果没有变换缓存，获取原始图片
	if result == nil {
		result, err = storageManager.GetWithLevel(fileID)
		if err != nil {
			log.Printf("获取文件失败: %v", err)
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
	}
	
	data := result.Data
	contentType := http.DetectContentType(data)
	
	// 如果需要变换且还没有变换
	if !isTransformed && (format != "" || widthStr != "" || heightStr != "") {
		// 解码原始图片
		img, imgFormat, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			log.Printf("解码图片失败: %v", err)
			http.Error(w, "Failed to decode image", http.StatusInternalServerError)
			return
		}
		
		// 应用尺寸调整
		if widthStr != "" || heightStr != "" {
			width, _ := strconv.Atoi(widthStr)
			height, _ := strconv.Atoi(heightStr)
			if mode == "" {
				mode = "fit"
			}
			img = resizeImage(img, width, height, mode)
		}
		
		// 编码为目标格式
		var buf bytes.Buffer
		targetFormat := format
		if targetFormat == "" && imgFormat != "gif" {
			targetFormat = "webp" // 默认转换为WebP
		}
		
		switch targetFormat {
		case "webp":
			if err := nativewebp.Encode(&buf, img, nil); err == nil {
				data = buf.Bytes()
				contentType = "image/webp"
			}
		case "png":
			if err := png.Encode(&buf, img); err == nil {
				data = buf.Bytes()
				contentType = "image/png"
			}
		case "jpeg", "jpg":
			quality := 85
			if q, err := strconv.Atoi(qualityStr); err == nil && q > 0 && q <= 100 {
				quality = q
			}
			if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err == nil {
				data = buf.Bytes()
				contentType = "image/jpeg"
			}
		default:
			// 保持原格式
			if targetFormat == "" && format == "webp" && imgFormat != "gif" {
				if err := nativewebp.Encode(&buf, img, nil); err == nil {
					data = buf.Bytes()
					contentType = "image/webp"
				}
			}
		}
		
		// 缓存变换后的图片（异步）
		if buf.Len() > 0 {
			go func(key string, transformedData []byte) {
				metadata := map[string]string{
					"custom_id": key,  // 使用transformKey作为自定义ID
					"original_id": fileID,
					"transform": fmt.Sprintf("f=%s,w=%s,h=%s,m=%s,q=%s", 
						format, widthStr, heightStr, mode, qualityStr),
				}
				if storedID, err := storageManager.Store(transformedData, metadata); err == nil {
					log.Printf("缓存变换后的图片: %s (存储为: %s)", key, storedID)
				}
			}(transformKey, data)
		}
		
		// 更新缓存状态为TRANSFORM
		result.CacheLevel = "Transform"
	}
	
	// 设置响应头
	w.Header().Set("Cache-Control", "public, max-age=31536000")
	w.Header().Set("ETag", generateETag(data))
	w.Header().Set("X-Cache-Level", result.CacheLevel)  // 缓存层级
	w.Header().Set("X-Storage-ID", fileID)              // 原始存储ID
	
	// 如果有变换，添加变换信息
	if transformKey != fileID {
		w.Header().Set("X-Transform-Key", transformKey)
		w.Header().Set("X-Transform-Params", fmt.Sprintf("format=%s,w=%s,h=%s,mode=%s,q=%s", 
			format, widthStr, heightStr, mode, qualityStr))
	}
	
	// 根据缓存层级设置状态
	switch result.CacheLevel {
	case "Memory":
		if isTransformed {
			w.Header().Set("X-Cache-Status", "HIT-MEMORY-TRANSFORM")
		} else {
			w.Header().Set("X-Cache-Status", "HIT-MEMORY")
		}
	case "Local":
		if isTransformed {
			w.Header().Set("X-Cache-Status", "HIT-LOCAL-TRANSFORM")
		} else {
			w.Header().Set("X-Cache-Status", "HIT-LOCAL")
		}
	case "IOBackend":
		w.Header().Set("X-Cache-Status", "HIT-REMOTE")
	case "Transform":
		w.Header().Set("X-Cache-Status", "TRANSFORM-ON-DEMAND")
	default:
		w.Header().Set("X-Cache-Status", "MISS")
	}
	
	// 检查ETag
	if match := r.Header.Get("If-None-Match"); match != "" {
		if match == w.Header().Get("ETag") {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	
	// 返回文件内容
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	
	// 添加图片尺寸信息（如果可用）
	if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
		bounds := img.Bounds()
		w.Header().Set("X-Image-Width", strconv.Itoa(bounds.Dx()))
		w.Header().Set("X-Image-Height", strconv.Itoa(bounds.Dy()))
	}
	
	w.Write(data)
}

// handleIOFiles 处理从 io 后端获取文件的请求（兼容旧接口）
func handleIOFiles(w http.ResponseWriter, r *http.Request) {
	// 获取文件路径
	path := strings.TrimPrefix(r.URL.Path, "/io/")
	if path == "" {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	
	// 提取 SHA1 哈希（去掉扩展名）
	sha1Hash := path
	if idx := strings.LastIndex(path, "."); idx > 0 {
		sha1Hash = path[:idx]
	}
	
	// 从 io 后端获取文件
	data, err := getFromIOBackend(sha1Hash)
	if err != nil {
		log.Printf("从 io 后端获取文件失败: %v", err)
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	
	// 检查是否需要转换为WebP
	format := r.URL.Query().Get("format")
	contentType := http.DetectContentType(data)
	
	if format == "webp" {
		// 如果不是WebP且不是GIF，则转换
		if contentType != "image/webp" && contentType != "image/gif" {
			// 解码图片
			img, _, err := image.Decode(bytes.NewReader(data))
			if err == nil {
				// 编码为WebP
				var buf bytes.Buffer
				if err := nativewebp.Encode(&buf, img, nil); err == nil {
					data = buf.Bytes()
					contentType = "image/webp"
				}
			}
		}
	}
	
	// 设置缓存头
	w.Header().Set("Cache-Control", "public, max-age=31536000")
	w.Header().Set("ETag", generateETag(data))
	
	// 检查ETag
	if match := r.Header.Get("If-None-Match"); match != "" {
		if match == w.Header().Get("ETag") {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	
	// 返回文件内容
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

// handleUploads 提供上传的图片访问
func handleUploads(w http.ResponseWriter, r *http.Request) {
	// 获取文件名
	filename := strings.TrimPrefix(r.URL.Path, "/uploads/")
	if filename == "" {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	
	// 构建文件路径
	filePath := filepath.Join("uploads", filename)
	
	// 安全检查：确保路径不会越界
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}
	uploadsAbsPath, _ := filepath.Abs("uploads")
	if !strings.HasPrefix(absPath, uploadsAbsPath) {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}
	
	// 检查文件是否存在
	fileInfo, err := os.Stat(filePath)
	if err != nil || fileInfo.IsDir() {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	
	// 读取文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}
	
	// 检查是否需要转换为WebP
	format := r.URL.Query().Get("format")
	if format == "webp" {
		// 检测当前格式
		contentType := http.DetectContentType(data)
		
		// 如果不是WebP且不是GIF，则转换
		if contentType != "image/webp" && contentType != "image/gif" {
			// 解码图片
			img, _, err := image.Decode(bytes.NewReader(data))
			if err == nil {
				// 编码为WebP
				var buf bytes.Buffer
				if err := nativewebp.Encode(&buf, img, nil); err == nil {
					data = buf.Bytes()
					contentType = "image/webp"
				}
			}
		}
	}
	
	// 设置缓存头
	w.Header().Set("Cache-Control", "public, max-age=31536000")
	w.Header().Set("ETag", generateETag(data))
	
	// 检查ETag
	if match := r.Header.Get("If-None-Match"); match != "" {
		if match == w.Header().Get("ETag") {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	
	// 返回文件内容
	contentType := http.DetectContentType(data)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

// Get 从LRU缓存获取条目
func (c *LRUCache) Get(key string) (*CacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	entry, exists := c.entries[key]
	if !exists {
		return nil, false
	}
	
	// 移动到链表头部（最近使用）
	c.moveToHead(entry)
	entry.AccessCount++
	entry.LastAccess = time.Now()
	entry.Dirty = true
	
	return entry, true
}

// Put 添加或更新LRU缓存条目
func (c *LRUCache) Put(key string, entry *CacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// 如果已存在，更新并移到头部
	if existing, exists := c.entries[key]; exists {
		c.currentSize -= existing.Size
		c.currentSize += entry.Size
		existing.FilePath = entry.FilePath
		existing.ThumbPath = entry.ThumbPath
		existing.Format = entry.Format
		existing.Size = entry.Size
		existing.LastAccess = time.Now()
		existing.Dirty = true
		c.moveToHead(existing)
		return
	}
	
	// 新条目
	c.entries[key] = entry
	c.currentSize += entry.Size
	c.addToHead(entry)
	
	// 检查是否超过限制，如果超过则淘汰
	for (len(c.entries) > c.maxEntries || c.currentSize > int64(c.maxSizeMB)*1024*1024) && c.tail != nil {
		c.evictTail()
	}
}

// moveToHead 移动节点到链表头部
func (c *LRUCache) moveToHead(entry *CacheEntry) {
	c.removeFromList(entry)
	c.addToHead(entry)
}

// addToHead 添加节点到链表头部
func (c *LRUCache) addToHead(entry *CacheEntry) {
	entry.prev = nil
	entry.next = c.head
	
	if c.head != nil {
		c.head.prev = entry
	}
	c.head = entry
	
	if c.tail == nil {
		c.tail = entry
	}
}

// removeFromList 从链表中移除节点
func (c *LRUCache) removeFromList(entry *CacheEntry) {
	if entry.prev != nil {
		entry.prev.next = entry.next
	} else {
		c.head = entry.next
	}
	
	if entry.next != nil {
		entry.next.prev = entry.prev
	} else {
		c.tail = entry.prev
	}
}

// evictTail 淘汰最久未使用的条目
func (c *LRUCache) evictTail() {
	if c.tail == nil {
		return
	}
	
	toEvict := c.tail
	delete(c.entries, toEvict.URL)
	c.currentSize -= toEvict.Size
	c.removeFromList(toEvict)
	
	// 删除文件
	if toEvict.FilePath != "" {
		os.Remove(toEvict.FilePath)
	}
	if toEvict.ThumbPath != "" {
		os.Remove(toEvict.ThumbPath)
	}
	
	log.Printf("LRU淘汰缓存: %s (大小: %d bytes)", toEvict.URL, toEvict.Size)
}

// GetAll 获取所有缓存条目（用于同步到数据库）
func (c *LRUCache) GetAll() map[string]*CacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	result := make(map[string]*CacheEntry)
	for k, v := range c.entries {
		result[k] = v
	}
	return result
}

// Len 返回缓存条目数量
func (c *LRUCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Remove 删除指定的缓存条目
func (c *LRUCache) Remove(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if entry, exists := c.entries[key]; exists {
		delete(c.entries, key)
		c.currentSize -= entry.Size
		c.removeFromList(entry)
		
		// 删除文件
		if entry.FilePath != "" {
			os.Remove(entry.FilePath)
		}
		if entry.ThumbPath != "" {
			os.Remove(entry.ThumbPath)
		}
	}
}
