# Building from Source / 从源码编译

This document provides instructions for building the WebP Image Proxy Service from source code.

本文档提供从源代码构建 WebP 图片代理服务的说明。

## Prerequisites / 前置要求

### Required Software / 必需软件

- **Go 1.21+** (Recommended: Go 1.24.4)
- **Git** for cloning the repository / 用于克隆仓库

### System Requirements / 系统要求

- Any OS that supports Go (Linux, macOS, Windows)
- At least 512MB RAM for compilation
- About 100MB disk space for source code and dependencies

## Build Instructions / 编译说明

### 1. Clone Repository / 克隆仓库

```bash
git clone https://github.com/BBOXAI/Images.git
cd Images
```

### 2. Install Dependencies / 安装依赖

```bash
go mod tidy
```

This will download all required Go modules / 这将下载所有必需的 Go 模块:
- `github.com/HugoSmits86/nativewebp` - WebP encoding library / WebP 编码库
- `modernc.org/sqlite` - Pure Go SQLite driver / 纯 Go SQLite 驱动

### 3. Build the Binary / 编译二进制文件

#### Standard Build / 标准编译

```bash
go build -o webpimg main.go
```

#### Build with Optimization / 优化编译

```bash
go build -ldflags="-s -w" -o webpimg main.go
```

The `-ldflags="-s -w"` flags reduce binary size by stripping debug information.

`-ldflags="-s -w"` 标志通过剥离调试信息来减小二进制文件大小。

### 4. Cross-Compilation / 交叉编译

#### Linux AMD64

```bash
GOOS=linux GOARCH=amd64 go build -o webpimg-linux-amd64 main.go
```

#### Windows AMD64

```bash
GOOS=windows GOARCH=amd64 go build -o webpimg-windows-amd64.exe main.go
```

#### macOS AMD64

```bash
GOOS=darwin GOARCH=amd64 go build -o webpimg-darwin-amd64 main.go
```

#### macOS ARM64 (M1/M2)

```bash
GOOS=darwin GOARCH=arm64 go build -o webpimg-darwin-arm64 main.go
```

#### Linux ARM64

```bash
GOOS=linux GOARCH=arm64 go build -o webpimg-linux-arm64 main.go
```

### 5. Build Script / 构建脚本

For convenience, you can create a build script / 为方便起见，你可以创建一个构建脚本:

#### build.sh (Linux/macOS)

```bash
#!/bin/bash

# Clean old builds
rm -f webpimg*

# Build for multiple platforms
echo "Building for Linux AMD64..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o webpimg-linux-amd64 main.go

echo "Building for Windows AMD64..."
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o webpimg-windows-amd64.exe main.go

echo "Building for macOS AMD64..."
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o webpimg-darwin-amd64 main.go

echo "Building for macOS ARM64..."
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o webpimg-darwin-arm64 main.go

echo "Build complete!"
ls -lh webpimg*
```

Make it executable / 使其可执行:
```bash
chmod +x build.sh
./build.sh
```

#### build.bat (Windows)

```batch
@echo off

REM Clean old builds
del webpimg*.exe 2>nul
del webpimg-linux-* 2>nul

REM Build for Windows
echo Building for Windows AMD64...
go build -ldflags="-s -w" -o webpimg-windows-amd64.exe main.go

REM Build for Linux
echo Building for Linux AMD64...
set GOOS=linux
set GOARCH=amd64
go build -ldflags="-s -w" -o webpimg-linux-amd64 main.go

REM Reset environment variables
set GOOS=
set GOARCH=

echo Build complete!
dir webpimg*
```

## Verification / 验证

After building, verify the binary works / 编译后，验证二进制文件是否正常工作:

```bash
./webpimg --version
# or
./webpimg
```

The service should start and display:
```
Server started on http://localhost:8080
Cache management: http://localhost:8080/cache
```

## Development Setup / 开发设置

### IDE Setup / IDE 设置

For development, we recommend using Visual Studio Code or GoLand with Go extensions.

开发时，我们建议使用带有 Go 扩展的 Visual Studio Code 或 GoLand。

### Running in Development / 开发运行

```bash
go run main.go
```

### Hot Reload / 热重载

Install air for hot reload during development / 安装 air 用于开发时热重载:

```bash
go install github.com/air-verse/air@latest
air
```

Create `.air.toml` configuration / 创建 `.air.toml` 配置:

```toml
root = "."
tmp_dir = "tmp"

[build]
  cmd = "go build -o ./tmp/main ."
  bin = "tmp/main"
  full_bin = "./tmp/main"
  include_ext = ["go", "tpl", "tmpl", "html"]
  exclude_dir = ["cache", "tmp", "vendor"]
  include_dir = []
  exclude_file = []
  delay = 1000
  stop_on_error = true
  log = "air_errors.log"

[log]
  time = true

[color]
  main = "magenta"
  watcher = "cyan"
  build = "yellow"
  runner = "green"
```

## Troubleshooting / 故障排除

### Common Issues / 常见问题

1. **Module download failed / 模块下载失败**
   
   Set Go proxy / 设置 Go 代理:
   ```bash
   go env -w GOPROXY=https://goproxy.cn,direct
   ```

2. **Build failed with "cannot find package" / 构建失败 "cannot find package"**
   
   Ensure Go modules are enabled / 确保 Go 模块已启用:
   ```bash
   go env -w GO111MODULE=on
   ```

3. **Binary too large / 二进制文件太大**
   
   Use build flags to reduce size / 使用构建标志减小大小:
   ```bash
   go build -ldflags="-s -w" -o webpimg main.go
   upx webpimg  # Optional: use UPX for further compression
   ```

### Performance Optimization / 性能优化

For production builds, consider / 对于生产构建，考虑:

```bash
go build -ldflags="-s -w" \
         -tags netgo \
         -a -installsuffix netgo \
         -o webpimg main.go
```

This creates a statically linked binary with better performance.

这将创建一个具有更好性能的静态链接二进制文件。

## Docker Build / Docker 构建

### Dockerfile

```dockerfile
# Build stage
FROM golang:1.24.4-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o webpimg main.go

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/webpimg .

EXPOSE 8080
CMD ["./webpimg"]
```

### Build Docker Image / 构建 Docker 镜像

```bash
docker build -t webpimg:latest .
```

### Run Docker Container / 运行 Docker 容器

```bash
docker run -d \
  --name webpimg \
  -p 8080:8080 \
  -v $(pwd)/cache:/root/cache \
  -v $(pwd)/imgproxy.db:/root/imgproxy.db \
  webpimg:latest
```

## CI/CD Integration / CI/CD 集成

The repository includes GitHub Actions workflow for automated builds.

仓库包含用于自动构建的 GitHub Actions 工作流。

See `.github/workflows/build.yml` for the configuration.

查看 `.github/workflows/build.yml` 了解配置。

## Testing / 测试

### Run Tests / 运行测试

```bash
go test ./...
```

### Run with Coverage / 运行覆盖率测试

```bash
go test -cover ./...
```

### Benchmark / 基准测试

```bash
go test -bench=. ./...
```

## Contributing / 贡献

When contributing code / 贡献代码时:

1. Fork the repository / Fork 仓库
2. Create your feature branch / 创建功能分支
3. Run tests / 运行测试
4. Submit pull request / 提交拉取请求

## License / 许可证

MIT License - See LICENSE file for details.

MIT 许可证 - 详见 LICENSE 文件。