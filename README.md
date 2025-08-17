# WebP Image Proxy Service

一个高性能的图片代理服务，自动将远程图片转换为WebP格式并提供缓存功能。

[English](#english) | [中文](#中文)

## 中文

### 功能特性

- 🚀 自动将图片转换为WebP格式，大幅减小文件体积
- 💾 三层存储架构：内存→本地→远程（可配置）
- 📊 实时统计和可视化管理界面
- 🔒 密码保护的管理后台
- 🌐 中英双语支持
- 📱 响应式界面设计
- ⚡ 高并发支持，内存缓存可减少数据库压力
- 🎨 支持图片缩放和多种调整模式
- 📤 图片上传功能，支持拖拽和批量上传
- 🔄 集成 [zots0127/io](https://github.com/zots0127/io) 去重存储后端（可选）
- 🏷️ 智能变换缓存：基于参数组合的缓存键，避免重复处理
- 📈 缓存层级信息：HTTP响应头显示数据来源（Memory/Local/Remote）

### 快速部署

#### 🚀 一键安装（推荐）

**Linux/macOS:**
```bash
curl -fsSL https://raw.githubusercontent.com/BBOXAI/Images/main/install.sh | sudo bash
```

**Windows (PowerShell 管理员模式):**
```powershell
irm https://raw.githubusercontent.com/BBOXAI/Images/main/install.ps1 | iex
```

安装脚本会自动：
- ✅ 检测系统架构并下载对应版本
- ✅ 创建系统服务并设置开机自启
- ✅ 生成管理密码和配置文件
- ✅ 配置防火墙规则
- ✅ 启动服务

#### 手动安装

1. **下载最新版本**

   访问 [Releases](https://github.com/BBOXAI/Images/releases) 页面下载适合你系统的版本：
   
   支持的平台：
   - Linux: `amd64`, `arm64`, `armv7`
   - Windows: `amd64`, `arm64`
   - macOS: `amd64`, `arm64`

2. **解压并运行**

   Linux/macOS:
   ```bash
   tar -xzf webpimg-linux-amd64.tar.gz
   chmod +x webpimg-linux-amd64
   ./webpimg-linux-amd64
   ```
   
   Windows:
   ```cmd
   # 解压 zip 文件后运行
   webpimg.exe
   ```

3. **访问服务**

   - 图片代理: `http://localhost:8080/[图片URL]`
   - 管理界面: `http://localhost:8080/cache`
   - 统计信息: `http://localhost:8080/stats`
   - 上传界面: `http://localhost:8080/upload`

### 存储配置

支持灵活的三层存储架构配置：

```bash
# 默认配置（内存+本地）
./webpimg

# 仅内存缓存
STORAGE_MEMORY=true STORAGE_LOCAL=false ./webpimg

# 完整三层存储（内存+本地+远程）
STORAGE_MEMORY=true \
STORAGE_LOCAL=true \
STORAGE_REMOTE=true \
./webpimg

# 自定义远程存储配置
STORAGE_REMOTE=true \
STORAGE_REMOTE_URL=http://your-io-backend:7777 \
STORAGE_REMOTE_APIKEY=your-api-key \
./webpimg
```

存储策略说明：
- **内存层**：最快速度，适合热点数据缓存
- **本地层**：本地磁盘存储，持久化保存
- **远程层**：集成 io 后端，支持去重和分布式存储
- **自动缓存**：数据会自动从慢层缓存到快层

#### 服务管理

**Linux (systemd):**
```bash
sudo systemctl status webpimg   # 查看状态
sudo systemctl stop webpimg     # 停止服务
sudo systemctl start webpimg    # 启动服务
sudo systemctl restart webpimg  # 重启服务
sudo journalctl -u webpimg -f   # 查看日志
```

**Windows:**
```powershell
Get-Service WebPImageProxy       # 查看状态
Stop-Service WebPImageProxy      # 停止服务
Start-Service WebPImageProxy     # 启动服务
Restart-Service WebPImageProxy   # 重启服务
```

#### 卸载

```bash
# Linux/macOS
sudo bash install.sh uninstall

# Windows (PowerShell 管理员模式)
.\install.ps1 uninstall
```

#### 更新

```bash
# Linux/macOS
sudo bash install.sh update

# Windows (PowerShell 管理员模式)
.\install.ps1 update
```

### 配置说明

#### 密码设置

服务首次启动时会自动生成8位随机密码并保存到 `.pass` 文件。你也可以手动创建：

```bash
echo "your-password" > .pass
```

#### 配置文件

服务会自动生成 `config.json` 配置文件，可通过管理界面修改或直接编辑：

```json
{
  "max_mem_cache_entries": 500,      // 内存缓存最大条目数
  "max_mem_cache_size_mb": 30,       // 内存缓存最大大小(MB)
  "max_disk_cache_size_mb": 200,     // 磁盘缓存最大大小(MB)
  "cleanup_interval_min": 10,        // 清理间隔(分钟)
  "access_window_min": 60,           // 访问时间窗口(分钟)
  "sync_interval_sec": 60,           // 数据库同步间隔(秒)
  "cache_validity_min": 15           // 缓存有效期(分钟)
}
```

### 使用方法

#### 基本使用

```
http://localhost:8080/https://example.com/image.jpg
```

#### 参数支持

- **格式转换**: `?format=webp` 或 `?format=original`
- **尺寸调整**: `?w=300&h=200`
- **质量设置**: `?q=85` (1-100)
- **调整模式**: `?mode=fit|fill|stretch|pad`
  - `fit`: 保持比例，缩放到指定范围内
  - `fill`: 保持比例，填充整个区域（可能裁剪）
  - `stretch`: 拉伸图片到指定尺寸
  - `pad`: 保持比例，用白色填充空白区域

#### 示例

```
# 转换为WebP并调整为300x200
http://localhost:8080/https://example.com/image.jpg?format=webp&w=300&h=200

# 保持原格式，质量85%
http://localhost:8080/https://example.com/image.jpg?format=original&q=85

# 使用填充模式调整尺寸
http://localhost:8080/https://example.com/image.jpg?w=400&h=300&mode=fill
```

### 从源码编译

如需自行编译，请参考 [BUILDING.md](BUILDING.md) 文档。

---

## English

### Features

- 🚀 Automatically converts images to WebP format, significantly reducing file size
- 💾 Three-tier storage architecture: Memory → Local → Remote (configurable)
- 📊 Real-time statistics and visual management interface
- 🔒 Password-protected admin panel
- 🌐 Bilingual support (Chinese/English)
- 📱 Responsive interface design
- ⚡ High concurrency support with memory cache to reduce database load
- 🎨 Image resizing with multiple adjustment modes
- 📤 Image upload feature with drag-and-drop and batch upload support
- 🔄 Integration with [zots0127/io](https://github.com/zots0127/io) deduplication storage backend (optional)
- 🏷️ Smart transform caching: Parameter-based cache keys to avoid redundant processing
- 📈 Cache level information: HTTP response headers show data source (Memory/Local/Remote)

### Quick Deployment

#### 🚀 One-Click Installation (Recommended)

**Linux/macOS:**
```bash
curl -fsSL https://raw.githubusercontent.com/BBOXAI/Images/main/install.sh | sudo bash
```

**Windows (PowerShell as Administrator):**
```powershell
irm https://raw.githubusercontent.com/BBOXAI/Images/main/install.ps1 | iex
```

The installation script will automatically:
- ✅ Detect system architecture and download the appropriate version
- ✅ Create system service with auto-start on boot
- ✅ Generate admin password and configuration files
- ✅ Configure firewall rules
- ✅ Start the service

#### Manual Installation

1. **Download Latest Release**

   Visit [Releases](https://github.com/BBOXAI/Images/releases) page to download the version for your system:
   
   Supported platforms:
   - Linux: `amd64`, `arm64`, `armv7`
   - Windows: `amd64`, `arm64`
   - macOS: `amd64`, `arm64`

2. **Extract and Run**

   Linux/macOS:
   ```bash
   tar -xzf webpimg-linux-amd64.tar.gz
   chmod +x webpimg-linux-amd64
   ./webpimg-linux-amd64
   ```
   
   Windows:
   ```cmd
   # Extract zip file and run
   webpimg.exe
   ```

3. **Access Service**

   - Image Proxy: `http://localhost:8080/[image-url]`
   - Admin Panel: `http://localhost:8080/cache`
   - Statistics: `http://localhost:8080/stats`
   - Upload Interface: `http://localhost:8080/upload`

#### Service Management

**Linux (systemd):**
```bash
sudo systemctl status webpimg   # Check status
sudo systemctl stop webpimg     # Stop service
sudo systemctl start webpimg    # Start service
sudo systemctl restart webpimg  # Restart service
sudo journalctl -u webpimg -f   # View logs
```

**Windows:**
```powershell
Get-Service WebPImageProxy       # Check status
Stop-Service WebPImageProxy      # Stop service
Start-Service WebPImageProxy     # Start service
Restart-Service WebPImageProxy   # Restart service
```

#### Uninstall

```bash
# Linux/macOS
sudo bash install.sh uninstall

# Windows (PowerShell as Administrator)
.\install.ps1 uninstall
```

#### Update

```bash
# Linux/macOS
sudo bash install.sh update

# Windows (PowerShell as Administrator)
.\install.ps1 update
```

### Configuration

#### Password Setup

The service automatically generates an 8-character random password on first startup and saves it to `.pass` file. You can also create it manually:

```bash
echo "your-password" > .pass
```

#### Configuration File

The service automatically generates a `config.json` file, which can be modified through the admin interface or edited directly:

```json
{
  "max_mem_cache_entries": 500,      // Maximum memory cache entries
  "max_mem_cache_size_mb": 30,       // Maximum memory cache size (MB)
  "max_disk_cache_size_mb": 200,     // Maximum disk cache size (MB)
  "cleanup_interval_min": 10,        // Cleanup interval (minutes)
  "access_window_min": 60,           // Access time window (minutes)
  "sync_interval_sec": 60,           // Database sync interval (seconds)
  "cache_validity_min": 15           // Cache validity period (minutes)
}
```

### Usage

#### Basic Usage

```
http://localhost:8080/https://example.com/image.jpg
```

#### Parameters

- **Format Conversion**: `?format=webp` or `?format=original`
- **Size Adjustment**: `?w=300&h=200`
- **Quality Setting**: `?q=85` (1-100)
- **Adjustment Mode**: `?mode=fit|fill|stretch|pad`
  - `fit`: Maintain aspect ratio, scale within specified bounds
  - `fill`: Maintain aspect ratio, fill entire area (may crop)
  - `stretch`: Stretch image to specified dimensions
  - `pad`: Maintain aspect ratio, fill blank areas with white

#### Examples

```
# Convert to WebP and resize to 300x200
http://localhost:8080/https://example.com/image.jpg?format=webp&w=300&h=200

# Keep original format, 85% quality
http://localhost:8080/https://example.com/image.jpg?format=original&q=85

# Resize with fill mode
http://localhost:8080/https://example.com/image.jpg?w=400&h=300&mode=fill
```

### Building from Source

For building from source code, please refer to [BUILDING.md](BUILDING.md).

---

## API Reference

### Statistics

```bash
GET /stats
```

Returns JSON statistics data:
- Request statistics
- Cache hit rate
- Space savings statistics
- Format distribution
- Memory cache status
- Storage tier information

### Cache Management

```bash
GET /cache                 # Admin interface
GET /cache?page=1&page_size=20  # Paginated data
POST /cache/control?action=toggle  # Toggle memory cache
POST /cache/control?action=sync    # Sync to database immediately
```

### Storage Files

```bash
GET /storage/{file_id}     # Get stored file
GET /storage/{file_id}?w=200&h=200  # With transformation
```

### Upload

```bash
POST /api/upload           # Upload images
```

Form data:
- `images`: Multiple image files

Returns:
```json
{
  "message": "Successfully uploaded 2 images",
  "urls": ["/storage/abc123.jpg", "/storage/def456.png"],
  "duplicates": 0
}
```

### Response Headers

The service adds informative headers to responses:

- `X-Cache-Level`: Storage tier that served the request (Memory/Local/IOBackend/Transform)
- `X-Cache-Status`: Cache hit status
  - `HIT-MEMORY`: Served from memory cache
  - `HIT-MEMORY-TRANSFORM`: Transformed image from memory cache
  - `HIT-LOCAL`: Served from local disk
  - `HIT-LOCAL-TRANSFORM`: Transformed image from local disk
  - `HIT-REMOTE`: Served from remote backend
  - `TRANSFORM-ON-DEMAND`: Real-time transformation
  - `MISS`: Not found in any cache
- `X-Transform-Key`: Unique key for transformed images
- `X-Transform-Params`: Transformation parameters applied
- `X-Storage-ID`: Original file storage ID
- `X-Image-Width`: Image width in pixels
- `X-Image-Height`: Image height in pixels

## System Requirements

- **Port**: Default port 8080 (auto-tries 8081-8100 if occupied)
- **Disk Space**: Recommend at least 500MB for cache storage
- **Memory**: Recommend at least 256MB RAM

## Troubleshooting

### Port Already in Use

The service automatically tries ports 8080-8100. The startup log shows the actual port being used.

### Cache Cleanup

Cache files are automatically cleaned based on configured validity period. You can also manually delete files in the `cache/` directory.

### Database Lock

If you encounter database lock issues, delete `imgproxy.db-wal` and `imgproxy.db-shm` files and restart the service.

## License

MIT License

## Contributing

Issues and Pull Requests are welcome!

## Links

- [GitHub Repository](https://github.com/BBOXAI/Images)
- [Issue Tracker](https://github.com/BBOXAI/Images/issues)
- [Releases](https://github.com/BBOXAI/Images/releases)