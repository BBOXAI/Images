# 依赖管理策略

## io 包版本管理

### 当前策略
- **当前版本**: v0.0.3
- **更新策略**: 手动审核更新
- **自动化**: Dependabot 每周检查并创建 PR

### 更新方式

#### 1. 自动更新检查（推荐）
Dependabot 会每周一早上 8:00 检查更新，如有新版本会自动创建 PR。

#### 2. 手动更新到最新版本
```bash
# 使用脚本
make update-io

# 或直接命令
GOPROXY=direct go get -u github.com/zots0127/io@latest
go mod tidy
```

#### 3. 更新到指定版本
```bash
GOPROXY=direct go get github.com/zots0127/io@v0.0.4
go mod tidy
```

#### 4. 检查所有依赖更新
```bash
make deps-check
```

## 版本选择建议

### 生产环境
- ✅ **使用指定版本**：确保稳定性和可重现性
- ✅ **经过测试后更新**：先在开发/测试环境验证

### 开发环境
- ✅ **可以使用最新版本**：获得最新功能
- ✅ **定期更新**：每周或每月更新一次

## 更新流程

1. **检查更新**
   ```bash
   make deps-check
   ```

2. **更新 io 包**
   ```bash
   make update-io
   ```

3. **运行测试**
   ```bash
   make test
   ```

4. **提交更改**
   ```bash
   git add go.mod go.sum
   git commit -m "deps: update io to vX.X.X"
   ```

## 自动化工具

### Dependabot
- 配置文件：`.github/dependabot.yml`
- 更新频率：每周
- 自动创建 PR
- 专门监控 `github.com/zots0127/io`

### GitHub Actions
- 每个 PR 自动运行测试
- 确保更新不会破坏现有功能

## 版本兼容性

### 语义化版本说明
- **v0.0.x**: 补丁版本，修复 bug
- **v0.x.0**: 次要版本，新功能（可能有破坏性变更）
- **vX.0.0**: 主要版本，重大变更

### 当前阶段（v0.0.x）
由于 io 包还在早期开发阶段（v0.0.x），需要注意：
- 可能会有频繁更新
- API 可能不稳定
- 建议及时跟进更新，但要充分测试

## 紧急回滚

如果更新后出现问题：

1. **查看之前的版本**
   ```bash
   git log --oneline go.mod
   ```

2. **回滚到之前版本**
   ```bash
   git checkout HEAD~1 go.mod go.sum
   go mod download
   ```

3. **或指定特定版本**
   ```bash
   GOPROXY=direct go get github.com/zots0127/io@v0.0.2
   go mod tidy
   ```

## 监控和通知

- **GitHub Watch**: 关注 https://github.com/zots0127/io 获取更新通知
- **Dependabot PRs**: 自动创建的 PR 会包含更新日志
- **Release Notes**: 查看每个版本的发布说明了解变更

## 决策矩阵

| 场景 | 建议策略 | 原因 |
|------|---------|------|
| 生产环境 | 指定版本 | 稳定性优先 |
| 开发环境 | 最新版本 | 获得新功能 |
| CI/CD | 指定版本 | 构建可重现 |
| 紧急修复 | 立即更新 | 安全优先 |
| 定期维护 | 按计划更新 | 平衡稳定与新功能 |