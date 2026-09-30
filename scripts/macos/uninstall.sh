#!/usr/bin/env bash
set -euo pipefail

# SlimBox macOS Native Edition - Service Uninstaller

echo "=========================================================="
echo "         SlimBox macOS 原生转码服务卸载程序               "
echo "=========================================================="

TARGET_PLIST="${HOME}/Library/LaunchAgents/com.slimbox.server.plist"
TARGET_BIN="${HOME}/.local/bin/slimbox"

if [[ -f "${TARGET_PLIST}" ]]; then
    echo "[1/3] 正在停止并注销 launchd 服务..."
    launchctl unload "${TARGET_PLIST}" 2>/dev/null || true
    rm -f "${TARGET_PLIST}"
else
    echo "[1/3] 未发现活跃的 launchd 服务。"
fi

if [[ -f "${TARGET_BIN}" ]]; then
    echo "[2/3] 正在删除程序文件: ${TARGET_BIN}..."
    rm -f "${TARGET_BIN}"
else
    echo "[2/3] 未找到安装的主程序。"
fi

echo "[3/3] 提示: 您的转码视频文件与数据库仍保留在原数据目录中，未被删除。"
echo "=========================================================="
echo "✅ SlimBox 服务已成功卸载！"
echo "=========================================================="
