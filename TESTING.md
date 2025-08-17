# 测试指南

## 快速开始

使用 Makefile 运行测试：

```bash
# 运行所有测试
make test

# 快速测试（不启动服务器）
make test-quick

# 集成测试
make test-integration

# 带覆盖率的测试
make test-coverage
```

## GitHub Actions 集成

项目配置了完整的 CI/CD 流程：

### 1. 持续测试 (`.github/workflows/test.yml`)
- **触发条件**: 每次推送到 main 分支或 Pull Request
- **测试矩阵**: 
  - OS: Ubuntu, macOS
  - Go 版本: 1.22, 1.23
- **测试内容**:
  - 代码检查 (vet, fmt)
  - 集成测试
  - 单元测试（如果存在）
  - 测试覆盖率报告

### 2. 构建和发布 (`.github/workflows/test-and-build.yml`)
- **触发条件**: 创建版本标签 (v*)
- **流程**:
  1. 运行完整测试套件
  2. 多平台编译（Windows, Linux, macOS）
  3. 创建 GitHub Release
  4. **测试报告自动附加到 Release 页面**

### 3. Pull Request 测试
- 自动运行测试并在 PR 中评论测试结果
- 设置状态检查，阻止未通过测试的合并

## 测试脚本

### `run_tests.sh`
基础测试脚本，包含以下测试：
- 健康检查
- 缓存页面访问
- 图片代理功能
- WebP 转换
- 缓存统计 API
- 缓存命中测试
- 参数化请求测试

### `test_suite.go`
Go 语言编写的完整测试套件：
```go
// 运行测试套件
go run test_suite.go
```

## 本地测试

### 准备环境
```bash
# 安装依赖
make deps

# 构建程序
make build

# 生成测试密码
echo "test123" > .pass
```

### 运行测试
```bash
# 方式1: 使用测试脚本
./run_tests.sh

# 方式2: 使用 Makefile
make test-integration

# 方式3: 手动测试
./webpimg &
curl http://localhost:8080/stats
```

## 测试报告

测试完成后会生成以下文件：
- `test-report.md` - Markdown 格式的测试报告
- `test-output.log` - 详细的测试输出日志
- `coverage.html` - HTML 格式的代码覆盖率报告（如果运行了覆盖率测试）

## 在 Release 页面查看测试结果

当创建新的版本发布时，GitHub Actions 会：
1. 自动运行所有测试
2. 生成测试报告
3. 将测试报告作为 Release 资产上传
4. 在 Release 描述中包含测试摘要

示例 Release 页面内容：
```markdown
## Release v1.0.0

### 🧪 Test Results
✅ Total Tests: 10
✅ Passed: 10
❌ Failed: 0
📊 Success Rate: 100%

### 📦 Downloads
[下载链接列表]

### 🔒 Checksums
[SHA256 校验和]
```

## 测试覆盖的功能

- ✅ 服务健康检查
- ✅ API 端点响应
- ✅ 图片代理功能
- ✅ WebP 格式转换
- ✅ 图片尺寸调整
- ✅ 图片质量控制
- ✅ 缓存命中率
- ✅ 缓存过期机制
- ✅ 管理界面访问
- ✅ 统计数据准确性

## 故障排除

### 端口被占用
```bash
# 使用环境变量指定端口
PORT=8081 make test-integration
```

### 测试失败调试
```bash
# 查看详细日志
cat test-output.log

# 手动运行特定测试
./webpimg &
curl -v http://localhost:8080/stats
```

### 清理测试环境
```bash
make clean
```

## 贡献指南

1. 添加新功能时，请同时添加相应的测试
2. 确保所有测试通过后再提交 PR
3. 测试覆盖率应保持在 70% 以上
4. 在 PR 描述中说明测试了哪些场景

## 性能基准测试

运行性能测试：
```bash
make benchmark
```

这将使用 Apache Bench (ab) 工具进行压力测试：
- 1000 个请求
- 10 个并发连接
- 测试图片代理性能