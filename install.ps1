# WebP Image Proxy Service - Windows 一键安装脚本
# https://github.com/BBOXAI/Images

param(
    [Parameter(Position=0)]
    [string]$Action = "install"
)

# 设置错误处理
$ErrorActionPreference = "Stop"

# 配置
$REPO = "BBOXAI/Images"
$SERVICE_NAME = "WebPImageProxy"
$INSTALL_DIR = "C:\Program Files\WebPImageProxy"
$GITHUB_API = "https://api.github.com/repos/$REPO/releases/latest"

# 颜色输出函数
function Write-ColorOutput {
    param(
        [string]$Message,
        [string]$Color = "White"
    )
    Write-Host $Message -ForegroundColor $Color
}

function Write-Success {
    param([string]$Message)
    Write-ColorOutput "✓ $Message" "Green"
}

function Write-Error {
    param([string]$Message)
    Write-ColorOutput "✗ $Message" "Red"
    exit 1
}

function Write-Info {
    param([string]$Message)
    Write-ColorOutput "→ $Message" "Cyan"
}

function Write-Warning {
    param([string]$Message)
    Write-ColorOutput "⚠ $Message" "Yellow"
}

# 检查管理员权限
function Test-Administrator {
    $currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
    return $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

# 检测系统架构
function Get-SystemArch {
    $arch = $env:PROCESSOR_ARCHITECTURE
    
    switch ($arch) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { Write-Error "不支持的架构: $arch" }
    }
}

# 获取最新版本
function Get-LatestRelease {
    Write-Info "获取最新版本信息..."
    
    try {
        $response = Invoke-RestMethod -Uri $GITHUB_API -Method Get
        $version = $response.tag_name
        
        if ([string]::IsNullOrEmpty($version)) {
            Write-Error "无法获取版本号"
        }
        
        Write-Success "最新版本: $version"
        return $version
    }
    catch {
        Write-Error "获取版本信息失败: $_"
    }
}

# 下载并安装
function Install-WebPImageProxy {
    param(
        [string]$Version,
        [string]$Arch
    )
    
    $assetName = "webpimg-windows-$Arch.zip"
    $downloadUrl = "https://github.com/$REPO/releases/download/$Version/$assetName"
    
    Write-Info "下载 $assetName..."
    
    # 创建临时目录
    $tempDir = New-TemporaryFile | % { Remove-Item $_; New-Item -ItemType Directory -Path $_ }
    $zipPath = Join-Path $tempDir $assetName
    
    try {
        # 下载文件
        Invoke-WebRequest -Uri $downloadUrl -OutFile $zipPath -UseBasicParsing
        Write-Success "下载完成"
        
        # 解压文件
        Write-Info "解压文件..."
        Expand-Archive -Path $zipPath -DestinationPath $tempDir -Force
        
        # 创建安装目录
        if (!(Test-Path $INSTALL_DIR)) {
            New-Item -ItemType Directory -Path $INSTALL_DIR -Force | Out-Null
        }
        
        # 复制文件
        $exePath = Join-Path $tempDir "webpimg.exe"
        if (!(Test-Path $exePath)) {
            # 尝试查找 exe 文件
            $exePath = Get-ChildItem -Path $tempDir -Filter "*.exe" -Recurse | Select-Object -First 1 | % { $_.FullName }
        }
        
        if (Test-Path $exePath) {
            Copy-Item -Path $exePath -Destination "$INSTALL_DIR\webpimg.exe" -Force
            Write-Success "程序已安装到 $INSTALL_DIR"
        }
        else {
            Write-Error "找不到可执行文件"
        }
    }
    finally {
        # 清理临时文件
        Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# 生成配置文件
function Initialize-Config {
    Write-Info "生成配置文件..."
    
    # 生成随机密码
    $passFile = Join-Path $INSTALL_DIR ".pass"
    if (!(Test-Path $passFile)) {
        $password = -join ((65..90) + (97..122) + (48..57) | Get-Random -Count 8 | % {[char]$_})
        $password | Out-File -FilePath $passFile -Encoding UTF8 -NoNewline
        Write-Success "管理密码已生成: $password"
        Write-Warning "请妥善保存此密码！"
    }
    else {
        Write-Info "密码文件已存在，跳过生成"
    }
    
    # 生成默认配置
    $configFile = Join-Path $INSTALL_DIR "config.json"
    if (!(Test-Path $configFile)) {
        $config = @{
            max_mem_cache_entries = 500
            max_mem_cache_size_mb = 30
            max_disk_cache_size_mb = 200
            cleanup_interval_min = 10
            access_window_min = 60
            sync_interval_sec = 60
            cache_validity_min = 15
        }
        
        $config | ConvertTo-Json -Depth 10 | Out-File -FilePath $configFile -Encoding UTF8
        Write-Success "配置文件已生成"
    }
}

# 创建 Windows 服务
function Install-WindowsService {
    Write-Info "创建 Windows 服务..."
    
    $exePath = Join-Path $INSTALL_DIR "webpimg.exe"
    
    # 检查服务是否已存在
    $service = Get-Service -Name $SERVICE_NAME -ErrorAction SilentlyContinue
    
    if ($service) {
        Write-Info "服务已存在，停止并删除旧服务..."
        Stop-Service -Name $SERVICE_NAME -Force -ErrorAction SilentlyContinue
        Start-Sleep -Seconds 2
        sc.exe delete $SERVICE_NAME | Out-Null
        Start-Sleep -Seconds 2
    }
    
    # 使用 NSSM 创建服务（如果可用）
    $nssmPath = Join-Path $INSTALL_DIR "nssm.exe"
    
    if (Test-Path $nssmPath) {
        # 使用 NSSM 创建服务
        & $nssmPath install $SERVICE_NAME $exePath
        & $nssmPath set $SERVICE_NAME AppDirectory $INSTALL_DIR
        & $nssmPath set $SERVICE_NAME DisplayName "WebP Image Proxy Service"
        & $nssmPath set $SERVICE_NAME Description "高性能图片代理服务，自动转换为WebP格式"
        & $nssmPath set $SERVICE_NAME Start SERVICE_AUTO_START
    }
    else {
        # 使用 sc.exe 创建服务
        $result = sc.exe create $SERVICE_NAME binPath= `"$exePath`" start= auto DisplayName= "WebP Image Proxy Service"
        
        if ($LASTEXITCODE -ne 0) {
            Write-Error "创建服务失败"
        }
        
        # 设置服务描述
        sc.exe description $SERVICE_NAME "高性能图片代理服务，自动转换为WebP格式" | Out-Null
    }
    
    # 设置服务恢复选项
    sc.exe failure $SERVICE_NAME reset= 86400 actions= restart/60000/restart/60000/restart/60000 | Out-Null
    
    Write-Success "Windows 服务已创建"
}

# 下载 NSSM (可选)
function Install-NSSM {
    Write-Info "下载 NSSM (服务管理工具)..."
    
    $nssmUrl = "https://nssm.cc/release/nssm-2.24.zip"
    $tempDir = New-TemporaryFile | % { Remove-Item $_; New-Item -ItemType Directory -Path $_ }
    $zipPath = Join-Path $tempDir "nssm.zip"
    
    try {
        Invoke-WebRequest -Uri $nssmUrl -OutFile $zipPath -UseBasicParsing
        Expand-Archive -Path $zipPath -DestinationPath $tempDir -Force
        
        $arch = Get-SystemArch
        $nssmExe = if ($arch -eq "amd64") { "win64\nssm.exe" } else { "win32\nssm.exe" }
        $nssmPath = Get-ChildItem -Path $tempDir -Filter "nssm.exe" -Recurse | 
                    Where-Object { $_.FullName -like "*$nssmExe*" } | 
                    Select-Object -First 1
        
        if ($nssmPath) {
            Copy-Item -Path $nssmPath.FullName -Destination "$INSTALL_DIR\nssm.exe" -Force
            Write-Success "NSSM 已安装"
        }
    }
    catch {
        Write-Warning "NSSM 下载失败，将使用 Windows 内置工具"
    }
    finally {
        Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# 配置防火墙
function Set-FirewallRule {
    Write-Info "配置防火墙规则..."
    
    # 删除旧规则
    Remove-NetFirewallRule -DisplayName "WebP Image Proxy" -ErrorAction SilentlyContinue
    
    # 添加新规则
    New-NetFirewallRule -DisplayName "WebP Image Proxy" `
                        -Direction Inbound `
                        -Protocol TCP `
                        -LocalPort 8080 `
                        -Action Allow `
                        -Profile Any | Out-Null
    
    Write-Success "防火墙规则已添加 (端口 8080)"
}

# 启动服务
function Start-WebPService {
    Write-Info "启动服务..."
    
    Start-Service -Name $SERVICE_NAME
    Start-Sleep -Seconds 3
    
    $service = Get-Service -Name $SERVICE_NAME
    if ($service.Status -eq "Running") {
        Write-Success "服务已启动"
    }
    else {
        Write-Error "服务启动失败"
    }
}

# 显示安装信息
function Show-InstallInfo {
    $ip = (Get-NetIPAddress -AddressFamily IPv4 -InterfaceAlias "Ethernet*","Wi-Fi*" | 
           Where-Object { $_.IPAddress -ne "127.0.0.1" } | 
           Select-Object -First 1).IPAddress
    
    if ([string]::IsNullOrEmpty($ip)) {
        $ip = "localhost"
    }
    
    $password = Get-Content -Path (Join-Path $INSTALL_DIR ".pass") -ErrorAction SilentlyContinue
    
    Write-Host ""
    Write-Success "=== 安装完成 ==="
    Write-Host ""
    Write-ColorOutput "服务状态:" "Green"
    Get-Service -Name $SERVICE_NAME | Format-Table -AutoSize
    Write-Host ""
    Write-ColorOutput "访问地址:" "Green"
    Write-Host "  图片代理: http://${ip}:8080/[图片URL]"
    Write-Host "  管理界面: http://${ip}:8080/cache"
    Write-Host "  统计信息: http://${ip}:8080/stats"
    Write-Host ""
    Write-ColorOutput "管理密码:" "Green"
    Write-Host "  $password"
    Write-Host ""
    Write-ColorOutput "常用命令:" "Green"
    Write-Host "  查看状态: Get-Service $SERVICE_NAME"
    Write-Host "  停止服务: Stop-Service $SERVICE_NAME"
    Write-Host "  启动服务: Start-Service $SERVICE_NAME"
    Write-Host "  重启服务: Restart-Service $SERVICE_NAME"
    Write-Host "  查看日志: Get-EventLog -LogName Application -Source $SERVICE_NAME"
    Write-Host "  卸载服务: .\install.ps1 uninstall"
    Write-Host ""
    
    # 添加到系统环境变量（可选）
    $currentPath = [Environment]::GetEnvironmentVariable("Path", "Machine")
    if ($currentPath -notlike "*$INSTALL_DIR*") {
        Write-Info "添加到系统 PATH..."
        [Environment]::SetEnvironmentVariable("Path", "$currentPath;$INSTALL_DIR", "Machine")
        Write-Success "已添加到系统 PATH"
    }
}

# 卸载服务
function Uninstall-WebPService {
    Write-Warning "开始卸载 WebP Image Proxy Service..."
    
    # 停止服务
    $service = Get-Service -Name $SERVICE_NAME -ErrorAction SilentlyContinue
    if ($service) {
        if ($service.Status -eq "Running") {
            Stop-Service -Name $SERVICE_NAME -Force
            Start-Sleep -Seconds 2
        }
        
        # 删除服务
        sc.exe delete $SERVICE_NAME | Out-Null
        Write-Success "服务已删除"
    }
    
    # 删除防火墙规则
    Remove-NetFirewallRule -DisplayName "WebP Image Proxy" -ErrorAction SilentlyContinue
    Write-Success "防火墙规则已删除"
    
    # 备份重要文件
    if (Test-Path $INSTALL_DIR) {
        $backupDir = "C:\Temp\webpimg-backup-$(Get-Date -Format 'yyyyMMdd-HHmmss')"
        New-Item -ItemType Directory -Path $backupDir -Force | Out-Null
        
        # 备份配置和数据
        $filesToBackup = @(".pass", "config.json", "imgproxy.db")
        foreach ($file in $filesToBackup) {
            $sourcePath = Join-Path $INSTALL_DIR $file
            if (Test-Path $sourcePath) {
                Copy-Item -Path $sourcePath -Destination $backupDir
            }
        }
        
        Write-Info "数据已备份到: $backupDir"
    }
    
    # 从 PATH 中移除
    $currentPath = [Environment]::GetEnvironmentVariable("Path", "Machine")
    if ($currentPath -like "*$INSTALL_DIR*") {
        $newPath = ($currentPath -split ';' | Where-Object { $_ -ne $INSTALL_DIR }) -join ';'
        [Environment]::SetEnvironmentVariable("Path", $newPath, "Machine")
        Write-Success "已从系统 PATH 中移除"
    }
    
    # 删除安装目录
    if (Test-Path $INSTALL_DIR) {
        Remove-Item -Path $INSTALL_DIR -Recurse -Force
        Write-Success "安装目录已删除"
    }
    
    Write-Success "卸载完成"
}

# 更新服务
function Update-WebPService {
    Write-Info "更新 WebP Image Proxy Service..."
    
    # 获取系统架构
    $arch = Get-SystemArch
    
    # 获取最新版本
    $version = Get-LatestRelease
    
    # 备份当前版本
    $exePath = Join-Path $INSTALL_DIR "webpimg.exe"
    if (Test-Path $exePath) {
        Copy-Item -Path $exePath -Destination "$exePath.backup" -Force
    }
    
    # 停止服务
    Stop-Service -Name $SERVICE_NAME -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
    
    # 下载新版本
    Install-WebPImageProxy -Version $version -Arch $arch
    
    # 启动服务
    Start-Service -Name $SERVICE_NAME
    
    Write-Success "更新完成"
}

# 主函数
function Main {
    Write-Host ""
    Write-Info "WebP Image Proxy Service 安装脚本"
    Write-Info "GitHub: https://github.com/BBOXAI/Images"
    Write-Host ""
    
    # 检查管理员权限
    if (!(Test-Administrator)) {
        Write-Error "此脚本需要管理员权限运行"
        Write-Host ""
        Write-Info "请右键点击脚本，选择'以管理员身份运行'"
        pause
        exit 1
    }
    
    switch ($Action.ToLower()) {
        "uninstall" {
            Uninstall-WebPService
        }
        "update" {
            Update-WebPService
        }
        default {
            # 获取系统架构
            $arch = Get-SystemArch
            Write-Info "检测到系统架构: Windows $arch"
            
            # 获取最新版本
            $version = Get-LatestRelease
            
            # 下载并安装
            Install-WebPImageProxy -Version $version -Arch $arch
            
            # 下载 NSSM（可选）
            # Install-NSSM
            
            # 生成配置
            Initialize-Config
            
            # 创建服务
            Install-WindowsService
            
            # 配置防火墙
            Set-FirewallRule
            
            # 启动服务
            Start-WebPService
            
            # 显示信息
            Show-InstallInfo
        }
    }
}

# 运行主函数
Main