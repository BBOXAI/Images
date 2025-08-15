#!/bin/bash

# WebP Image Proxy Service - 一键安装脚本
# https://github.com/BBOXAI/Images

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置
REPO="BBOXAI/Images"
SERVICE_NAME="webpimg"
INSTALL_DIR="/opt/webpimg"
SERVICE_USER="webpimg"
GITHUB_API="https://api.github.com/repos/${REPO}/releases/latest"

# 打印带颜色的消息
print_msg() {
    echo -e "${2}${1}${NC}"
}

print_error() {
    print_msg "✗ $1" "$RED"
    exit 1
}

print_success() {
    print_msg "✓ $1" "$GREEN"
}

print_info() {
    print_msg "→ $1" "$BLUE"
}

print_warning() {
    print_msg "⚠ $1" "$YELLOW"
}

# 检查是否为 root 用户
check_root() {
    if [[ $EUID -ne 0 ]]; then
        print_error "此脚本需要 root 权限运行，请使用 sudo 运行"
    fi
}

# 检测系统架构
detect_arch() {
    ARCH=$(uname -m)
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    
    case "$ARCH" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        armv7l|armhf)
            ARCH="armv7"
            ;;
        *)
            print_error "不支持的架构: $ARCH"
            ;;
    esac
    
    case "$OS" in
        linux)
            OS="linux"
            ;;
        darwin)
            OS="darwin"
            ;;
        *)
            print_error "不支持的操作系统: $OS"
            ;;
    esac
    
    PLATFORM="${OS}-${ARCH}"
    print_info "检测到系统: $OS, 架构: $ARCH"
}

# 检查必要的命令
check_dependencies() {
    local deps=("curl" "tar" "systemctl")
    
    for dep in "${deps[@]}"; do
        if ! command -v $dep &> /dev/null; then
            print_warning "$dep 未安装，正在安装..."
            
            if command -v apt-get &> /dev/null; then
                apt-get update && apt-get install -y $dep
            elif command -v yum &> /dev/null; then
                yum install -y $dep
            elif command -v brew &> /dev/null; then
                brew install $dep
            else
                print_error "无法安装 $dep，请手动安装"
            fi
        fi
    done
}

# 获取最新版本信息
get_latest_release() {
    print_info "获取最新版本信息..."
    
    RELEASE_INFO=$(curl -s $GITHUB_API)
    
    if [ -z "$RELEASE_INFO" ]; then
        print_error "无法获取版本信息"
    fi
    
    VERSION=$(echo "$RELEASE_INFO" | grep -oP '"tag_name":\s*"\K[^"]+' | head -1)
    
    if [ -z "$VERSION" ]; then
        print_error "无法解析版本号"
    fi
    
    print_success "最新版本: $VERSION"
}

# 下载并安装
download_and_install() {
    local ASSET_NAME="webpimg-${PLATFORM}.tar.gz"
    local DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_NAME}"
    
    print_info "下载 $ASSET_NAME..."
    
    # 创建临时目录
    TMP_DIR=$(mktemp -d)
    cd "$TMP_DIR"
    
    # 下载文件
    if ! curl -L -o "$ASSET_NAME" "$DOWNLOAD_URL"; then
        print_error "下载失败: $DOWNLOAD_URL"
    fi
    
    print_success "下载完成"
    
    # 解压文件
    print_info "解压文件..."
    tar -xzf "$ASSET_NAME"
    
    # 创建安装目录
    print_info "创建安装目录..."
    mkdir -p "$INSTALL_DIR"
    
    # 复制文件
    cp webpimg "$INSTALL_DIR/"
    chmod +x "$INSTALL_DIR/webpimg"
    
    # 清理临时文件
    cd /
    rm -rf "$TMP_DIR"
    
    print_success "程序已安装到 $INSTALL_DIR"
}

# 创建系统用户
create_user() {
    if id "$SERVICE_USER" &>/dev/null; then
        print_info "用户 $SERVICE_USER 已存在"
    else
        print_info "创建系统用户 $SERVICE_USER..."
        useradd -r -s /bin/false -d "$INSTALL_DIR" "$SERVICE_USER"
        print_success "用户创建成功"
    fi
    
    # 设置目录权限
    chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR"
}

# 生成配置文件
generate_config() {
    print_info "生成配置文件..."
    
    # 生成随机密码
    if [ ! -f "$INSTALL_DIR/.pass" ]; then
        PASSWORD=$(openssl rand -base64 6 | tr -d "=+/")
        echo "$PASSWORD" > "$INSTALL_DIR/.pass"
        chmod 600 "$INSTALL_DIR/.pass"
        chown "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR/.pass"
        print_success "管理密码已生成: $PASSWORD"
        print_warning "请妥善保存此密码！"
    else
        print_info "密码文件已存在，跳过生成"
    fi
    
    # 生成默认配置
    if [ ! -f "$INSTALL_DIR/config.json" ]; then
        cat > "$INSTALL_DIR/config.json" <<EOF
{
  "max_mem_cache_entries": 500,
  "max_mem_cache_size_mb": 30,
  "max_disk_cache_size_mb": 200,
  "cleanup_interval_min": 10,
  "access_window_min": 60,
  "sync_interval_sec": 60,
  "cache_validity_min": 15
}
EOF
        chown "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR/config.json"
        print_success "配置文件已生成"
    fi
}

# 创建 systemd 服务
create_systemd_service() {
    print_info "创建 systemd 服务..."
    
    cat > /etc/systemd/system/webpimg.service <<EOF
[Unit]
Description=WebP Image Proxy Service
After=network.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/webpimg
Restart=always
RestartSec=10
StandardOutput=append:/var/log/webpimg/access.log
StandardError=append:/var/log/webpimg/error.log

# 安全设置
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$INSTALL_DIR /var/log/webpimg

# 资源限制
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
EOF
    
    # 创建日志目录
    mkdir -p /var/log/webpimg
    chown "$SERVICE_USER:$SERVICE_USER" /var/log/webpimg
    
    # 重新加载 systemd
    systemctl daemon-reload
    
    print_success "systemd 服务已创建"
}

# 配置防火墙
configure_firewall() {
    print_info "配置防火墙..."
    
    # 检查 firewalld
    if command -v firewall-cmd &> /dev/null && systemctl is-active firewalld &> /dev/null; then
        firewall-cmd --permanent --add-port=8080/tcp
        firewall-cmd --reload
        print_success "firewalld 规则已添加"
    # 检查 ufw
    elif command -v ufw &> /dev/null; then
        ufw allow 8080/tcp
        print_success "ufw 规则已添加"
    # 检查 iptables
    elif command -v iptables &> /dev/null; then
        iptables -A INPUT -p tcp --dport 8080 -j ACCEPT
        
        # 保存规则
        if command -v iptables-save &> /dev/null; then
            iptables-save > /etc/iptables/rules.v4
        fi
        print_success "iptables 规则已添加"
    else
        print_warning "未检测到防火墙，请手动开放 8080 端口"
    fi
}

# 启动服务
start_service() {
    print_info "启动服务..."
    
    systemctl enable webpimg
    systemctl start webpimg
    
    # 等待服务启动
    sleep 2
    
    if systemctl is-active webpimg &> /dev/null; then
        print_success "服务已启动"
    else
        print_error "服务启动失败，请检查日志: journalctl -u webpimg -n 50"
    fi
}

# 显示安装信息
show_info() {
    local IP=$(hostname -I | awk '{print $1}')
    
    echo ""
    print_success "=== 安装完成 ==="
    echo ""
    echo -e "${GREEN}服务状态:${NC}"
    systemctl status webpimg --no-pager | head -n 5
    echo ""
    echo -e "${GREEN}访问地址:${NC}"
    echo "  图片代理: http://${IP}:8080/[图片URL]"
    echo "  管理界面: http://${IP}:8080/cache"
    echo "  统计信息: http://${IP}:8080/stats"
    echo ""
    echo -e "${GREEN}管理密码:${NC}"
    echo "  $(cat $INSTALL_DIR/.pass)"
    echo ""
    echo -e "${GREEN}常用命令:${NC}"
    echo "  查看状态: systemctl status webpimg"
    echo "  停止服务: systemctl stop webpimg"
    echo "  启动服务: systemctl start webpimg"
    echo "  重启服务: systemctl restart webpimg"
    echo "  查看日志: journalctl -u webpimg -f"
    echo "  卸载服务: $0 uninstall"
    echo ""
}

# 卸载服务
uninstall() {
    print_warning "开始卸载 WebP Image Proxy Service..."
    
    # 停止服务
    if systemctl is-active webpimg &> /dev/null; then
        systemctl stop webpimg
        systemctl disable webpimg
    fi
    
    # 删除服务文件
    rm -f /etc/systemd/system/webpimg.service
    systemctl daemon-reload
    
    # 删除用户
    if id "$SERVICE_USER" &>/dev/null; then
        userdel "$SERVICE_USER"
    fi
    
    # 备份数据
    if [ -d "$INSTALL_DIR" ]; then
        BACKUP_DIR="/tmp/webpimg-backup-$(date +%Y%m%d-%H%M%S)"
        mkdir -p "$BACKUP_DIR"
        
        # 备份重要文件
        [ -f "$INSTALL_DIR/.pass" ] && cp "$INSTALL_DIR/.pass" "$BACKUP_DIR/"
        [ -f "$INSTALL_DIR/config.json" ] && cp "$INSTALL_DIR/config.json" "$BACKUP_DIR/"
        [ -f "$INSTALL_DIR/imgproxy.db" ] && cp "$INSTALL_DIR/imgproxy.db" "$BACKUP_DIR/"
        
        print_info "数据已备份到: $BACKUP_DIR"
    fi
    
    # 删除安装目录
    rm -rf "$INSTALL_DIR"
    rm -rf /var/log/webpimg
    
    print_success "卸载完成"
}

# 更新服务
update() {
    print_info "更新 WebP Image Proxy Service..."
    
    # 获取最新版本
    get_latest_release
    
    # 备份当前版本
    if [ -f "$INSTALL_DIR/webpimg" ]; then
        cp "$INSTALL_DIR/webpimg" "$INSTALL_DIR/webpimg.backup"
    fi
    
    # 停止服务
    systemctl stop webpimg
    
    # 下载新版本
    download_and_install
    
    # 设置权限
    chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR"
    
    # 启动服务
    systemctl start webpimg
    
    print_success "更新完成"
}

# 主函数
main() {
    echo ""
    print_info "WebP Image Proxy Service 安装脚本"
    print_info "GitHub: https://github.com/BBOXAI/Images"
    echo ""
    
    # 检查参数
    case "${1:-}" in
        uninstall)
            check_root
            uninstall
            ;;
        update)
            check_root
            detect_arch
            update
            ;;
        *)
            check_root
            detect_arch
            check_dependencies
            get_latest_release
            download_and_install
            create_user
            generate_config
            create_systemd_service
            configure_firewall
            start_service
            show_info
            ;;
    esac
}

# 运行主函数
main "$@"