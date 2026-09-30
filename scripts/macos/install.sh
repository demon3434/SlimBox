#!/usr/bin/env bash
set -euo pipefail

# SlimBox macOS Native Edition - Automated Headless Service Installer
# Registers SlimBox as a native launchd LaunchAgent for silent background execution.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PORT=8080
DATA_DIR=""

usage() {
    echo "SlimBox macOS 一键安装程序"
    echo "用法: ./install.sh [--data-dir <路径>] [--port <端口>]"
    echo ""
    echo "参数:"
    echo "  --data-dir   存储根目录 (强烈建议设置为外接 USB 硬盘，如 /Volumes/MyDisk/SlimBox)"
    echo "  --port       Web 服务端口 (默认: 8080)"
    echo "  -h, --help   显示此帮助信息"
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --data-dir)
            DATA_DIR="$2"
            shift 2
            ;;
        --port)
            PORT="$2"
            shift 2
            ;;
        -h|--help)
            usage
            ;;
        *)
            echo "未知参数: $1"
            usage
            ;;
    esac
done

echo "=========================================================="
echo "         SlimBox macOS 原生转码服务安装程序               "
echo "=========================================================="

# 1. 查找二进制程序
SLIMBOX_SRC=""
if [[ -f "${SCRIPT_DIR}/slimbox" ]]; then
    SLIMBOX_SRC="${SCRIPT_DIR}/slimbox"
elif [[ -f "${SCRIPT_DIR}/../../slimbox" ]]; then
    SLIMBOX_SRC="${SCRIPT_DIR}/../../slimbox"
else
    echo "[错误] 未在当前目录或上层目录找到 'slimbox' 执行程序。"
    exit 1
fi

# 2. 确定安装目标路径
INSTALL_BIN_DIR="${HOME}/.local/bin"
mkdir -p "${INSTALL_BIN_DIR}"
TARGET_BIN="${INSTALL_BIN_DIR}/slimbox"

echo "[1/5] 安装主程序到 ${TARGET_BIN}..."
cp -f "${SLIMBOX_SRC}" "${TARGET_BIN}"
chmod +x "${TARGET_BIN}"

# 如果有内置的 bin/ffmpeg，同步安装到 ~/.local/bin/
if [[ -d "${SCRIPT_DIR}/bin" ]]; then
    echo "[*] 同步内置静态依赖到 ${INSTALL_BIN_DIR}..."
    cp -rf "${SCRIPT_DIR}/bin/"* "${INSTALL_BIN_DIR}/" 2>/dev/null || true
fi

# 3. 移除 macOS Gatekeeper Quarantine 属性 & 配置防火墙
echo "[2/5] 清除隔离拦截并配置局域网访问权限..."
xattr -d com.apple.quarantine "${TARGET_BIN}" 2>/dev/null || true
codesign -s - -f "${TARGET_BIN}" 2>/dev/null || true
if [[ -x /usr/libexec/ApplicationFirewall/socketfilterfw ]]; then
    /usr/libexec/ApplicationFirewall/socketfilterfw --add "${TARGET_BIN}" 2>/dev/null || true
    /usr/libexec/ApplicationFirewall/socketfilterfw --unblockapp "${TARGET_BIN}" 2>/dev/null || true
fi

# 4. 确定存储路径
if [[ -z "${DATA_DIR}" ]]; then
    DATA_DIR="${HOME}/Library/Application Support/SlimBox/data"
    echo "[3/5] 未指定外接硬盘，使用默认数据路径: ${DATA_DIR}"
else
    echo "[3/5] 使用自定义/外接硬盘数据路径: ${DATA_DIR}"
fi
mkdir -p "${DATA_DIR}/uploads" "${DATA_DIR}/outputs"

# 5. 配置日志目录
LOG_DIR="${HOME}/Library/Logs/slimbox"
mkdir -p "${LOG_DIR}"

# 6. 配置 launchd 守护服务与管理命令
echo "[4/5] 注册 macOS launchd 系统服务 (com.slimbox.server)..."
PLIST_TEMPLATE="${SCRIPT_DIR}/com.slimbox.server.plist"
if [[ ! -f "${PLIST_TEMPLATE}" ]]; then
    echo "[错误] 缺少 plist 模板文件: ${PLIST_TEMPLATE}"
    exit 1
fi

LAUNCH_AGENTS_DIR="${HOME}/Library/LaunchAgents"
mkdir -p "${LAUNCH_AGENTS_DIR}"
TARGET_PLIST="${LAUNCH_AGENTS_DIR}/com.slimbox.server.plist"

# 替换模板变量
sed -e "s|{{SLIMBOX_BIN}}|${TARGET_BIN}|g" \
    -e "s|{{PORT}}|${PORT}|g" \
    -e "s|{{DATA_DIR}}|${DATA_DIR}|g" \
    -e "s|{{LOG_DIR}}|${LOG_DIR}|g" \
    "${PLIST_TEMPLATE}" > "${TARGET_PLIST}"

# 创建 slimbox-ctl 快捷命令行运维脚本
CTL_SCRIPT="${INSTALL_BIN_DIR}/slimbox-ctl"
cat << 'EOF' > "${CTL_SCRIPT}"
#!/usr/bin/env bash
PLIST="${HOME}/Library/LaunchAgents/com.slimbox.server.plist"
LOG="${HOME}/Library/Logs/slimbox/server-error.log"

case "$1" in
    start)
        launchctl load "${PLIST}" 2>/dev/null || nohup slimbox --port 8080 --data-dir /Volumes/SlimBoxUSB/SlimBox > "${LOG}" 2>&1 &
        echo "SlimBox 服务已启动"
        ;;
    stop)
        launchctl unload "${PLIST}" 2>/dev/null || true
        pkill -f "slimbox --port" || true
        echo "SlimBox 服务已停止"
        ;;
    restart)
        $0 stop
        sleep 1
        $0 start
        ;;
    status)
        ps aux | grep -v grep | grep "slimbox" || echo "服务未运行"
        ;;
    log)
        tail -f "${LOG}"
        ;;
    *)
        echo "用法: slimbox-ctl {start|stop|restart|status|log}"
        ;;
esac
EOF
chmod +x "${CTL_SCRIPT}"

# 7. 重载并启动服务
echo "[5/5] 启动后台守护服务..."
launchctl unload "${TARGET_PLIST}" 2>/dev/null || true
launchctl load "${TARGET_PLIST}" 2>/dev/null || true

# 获取局域网 IP
LAN_IP=""
if command -v ipconfig &>/dev/null; then
    LAN_IP=$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || true)
fi

echo "=========================================================="
echo "🎉 SlimBox macOS 原生转码服务已成功安装并在后台运行！"
echo ""
echo "  * 本地 Web 控制台:  http://localhost:${PORT}"
if [[ -n "${LAN_IP}" ]]; then
echo "  * 局域网访问地址:   http://${LAN_IP}:${PORT}"
fi
echo "  * 存储根目录:       ${DATA_DIR}"
echo "  * 运行与转码日志:   ${LOG_DIR}/server-error.log"
echo ""
echo "常用维护命令:"
echo "  快捷管理脚本:       slimbox-ctl {start|stop|restart|status|log}"
echo "  查看实时转码日志:   tail -f ${LOG_DIR}/server-error.log"
echo "  彻底卸载服务:       ${SCRIPT_DIR}/uninstall.sh"
echo "=========================================================="
