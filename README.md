# WebP Image Proxy / WebP图片代理

This is a Go program that acts as an online image proxy and converter. It fetches images from a given URL, converts them to WebP format, and serves them with intelligent caching and database storage.

这是一个Go程序，作为在线图片代理和转换器。它从给定的URL获取图片，转换为WebP格式，并提供智能缓存和数据库存储服务。

## Features / 功能特性

- **Image Proxy & Conversion / 图片代理与转换**: Fetch images from remote URLs and convert them in real-time / 从远程URL获取图片并实时转换
- **WebP Format Support / WebP格式支持**: Uses pure Go native library `github.com/HugoSmits86/nativewebp` for WebP encoding / 使用纯Go原生库 `github.com/HugoSmits86/nativewebp` 进行WebP编码
- **Smart Format Processing / 智能格式处理**: 
  - Static images (PNG, JPEG) → WebP format / 静态图片 (PNG, JPEG) → WebP格式
  - Animated GIF → Keep GIF format / 动态GIF → 保持GIF格式
- **High-Performance Caching / 高性能缓存**: Local file cache + SQLite database storage / 本地文件缓存 + SQLite数据库存储
- **Statistics / 统计功能**: Request count, cache hit rate, space and traffic savings statistics / 请求计数、缓存命中率、节省空间和流量统计
- **Cache Management / 缓存管理**: Visual cache list with sorting, filtering, and pagination / 可视化缓存列表，支持排序、筛选、分页浏览
- **Thumbnail Generation / 缩略图生成**: Automatically generate 200x200 pixel WebP thumbnails for preview / 自动生成200x200像素WebP缩略图用于预览
- **Pure Go Implementation / 纯Go实现**: No CGO dependencies, uses `modernc.org/sqlite` pure Go SQLite driver / 无CGO依赖，使用 `modernc.org/sqlite` 纯Go SQLite驱动
- **Automatic Cache Management / 自动缓存管理**: Supports cache size limits and automatic cleanup / 支持缓存大小限制和自动清理

## Tech Stack / 技术栈

- **Go 1.24.4**: Main programming language / 主要编程语言
- **github.com/HugoSmits86/nativewebp v0.9.3**: Pure Go WebP encoding library / 纯Go WebP编码库
- **modernc.org/sqlite v1.38.0**: Pure Go SQLite database driver / 纯Go SQLite数据库驱动
- **Standard Library / 标准库**: `image/png`, `image/gif`, `image/jpeg` for image decoding / 用于图片解码

## Installation & Usage / 安装与使用

### 1. Install Dependencies / 安装依赖
```bash
go mod tidy
```

### 2. Build Program / 构建程序
```bash
go build -o webp_proxy main.go
```

### 3. Run Program / 运行程序
```bash
./webp_proxy
```
Default server starts at `http://localhost:8080` / 默认服务器启动在 `http://localhost:8080`

### 4. Use Proxy / 使用代理

**Image conversion URL format / 图片转换URL格式:**
```
http://your-domain.com/<image_url>
```

**Example / 示例:**
```
http://localhost:8080/https://example.com/image.png
```

### 5. View Statistics / 查看统计

Access statistics page / 访问统计页面:
```
http://localhost:8080/stats
```

**Statistics include / 统计信息包括:**
- Total requests and current time / 总请求数和当前时间
- Cache file count, size, hit rate / 缓存文件数量、大小、命中率
- **Total space saved / 总节省空间**: Storage space saved through WebP compression / 通过WebP压缩节省的存储空间
- **Total traffic saved / 总节省流量**: Network transmission reduced through compression / 通过压缩减少的网络传输量
- Cache rules and usage instructions / 缓存规则和使用说明

### 6. Cache Management / 缓存管理

Access cache management page / 访问缓存管理页面:
```
http://localhost:8080/cache
```

**Cache management features / 缓存管理功能:**
- 📋 **List Display / 列表展示**: View all cached image files / 查看所有缓存的图片文件
- 🖼️ **Thumbnail Preview / 缩略图预览**: Auto-generate 200x200 pixel WebP thumbnails / 自动生成200x200像素的WebP缩略图
- 📊 **Sorting / 排序功能**: Sort by access count, last access time, creation time, URL / 支持按访问次数、最后访问时间、创建时间、URL排序
- 🔍 **Format Filtering / 格式筛选**: Filter by image format (WebP, GIF, PNG, JPEG) / 按图片格式（WebP、GIF、PNG、JPEG）筛选
- 📄 **Pagination / 分页浏览**: Custom items per page (1-100) / 支持自定义每页显示数量（1-100个）
- 📱 **Responsive Design / 响应式设计**: Adapts to desktop and mobile devices / 适配桌面和移动设备

## Output Format / 输出格式

| Input Format / 输入格式 | Output Format / 输出格式 | Description / 说明 |
|---------|---------|------|
| PNG | WebP | Static image converted to WebP / 静态图片转换为WebP |
| JPEG | WebP | Static image converted to WebP / 静态图片转换为WebP |
| Static GIF / 静态GIF | WebP | Single-frame GIF converted to WebP / 单帧GIF转换为WebP |
| Animated GIF / 动态GIF | GIF | Keep original format to support animation / 保持原格式以支持动画 |

## Cache Mechanism / 缓存机制

- **Local File Cache / 本地文件缓存**: Converted images stored in `cache/` directory / 转换后的图片存储在 `cache/` 目录
- **SQLite Database / SQLite数据库**: Store cache metadata and statistics / 存储缓存元数据和统计信息
- **Cache Key / 缓存键**: MD5 hash based on original URL / 基于原始URL的MD5哈希
- **Cache Validity / 缓存有效期**: Unified 10-minute validity period from last access time / 统一10分钟有效期，从最后访问时间开始计算
- **Auto Cleanup / 自动清理**: Periodically clean expired cache files and database records / 定期清理过期缓存文件和数据库记录

## Deployment Recommendations / 部署建议

It is recommended to use a reverse proxy (such as Nginx) in production environments to handle SSL termination and domain mapping.

建议在生产环境中使用反向代理（如Nginx）来处理SSL终止和域名映射。

**Nginx Configuration Example / Nginx配置示例:**

```nginx
server {
    listen 80;
    server_name your-domain.com;
    
    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Increase timeout for handling large images / 增加超时时间以处理大图片
        proxy_read_timeout 60s;
        proxy_connect_timeout 60s;
    }
}
```

## Performance Features / 性能特点

- **Pure Go Implementation / 纯Go实现**: No CGO dependencies, simple deployment / 无CGO依赖，部署简单
- **Efficient Caching / 高效缓存**: Avoid duplicate conversions, improve response speed / 避免重复转换，提升响应速度
- **Memory Optimization / 内存优化**: Stream processing for large images, control memory usage / 流式处理大图片，控制内存使用
- **Concurrency Safe / 并发安全**: Support multiple concurrent request processing / 支持多并发请求处理

## Notes / 注意事项

1. **Network Dependency / 网络依赖**: First access requires downloading images from source / 首次访问需要从源站下载图片
2. **Storage Space / 存储空间**: Cache will occupy local disk space / 缓存会占用本地磁盘空间
3. **Animated GIF / 动态GIF**: To maintain animation effects, animated GIFs keep original format / 为保持动画效果，动态GIF保持原格式
4. **Error Handling / 错误处理**: Returns 404 error when source image is inaccessible / 源图片无法访问时返回404错误

## Development Status / 开发状态

Current version implemented / 当前版本已实现:
- ✅ Static image WebP conversion / 静态图片WebP转换
- ✅ Animated GIF format preservation / 动态GIF格式保持
- ✅ SQLite cache system / SQLite缓存系统
- ✅ Statistics (request count, hit rate, space savings) / 统计功能（请求数、命中率、节省空间统计）
- ✅ Cache management page (list, sort, filter, pagination) / 缓存管理页面（列表、排序、筛选、分页）
- ✅ Automatic thumbnail generation and preview / 缩略图自动生成和预览
- ❌ Animated GIF to WebP animation (temporarily removed due to library compatibility issues) / 动态GIF转WebP动画（因库兼容性问题暂时移除）

## License / 许可证

MIT License