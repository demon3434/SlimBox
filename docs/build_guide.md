# SlimBox 编译与构建指南

本文档介绍如何从源码自行编译各平台单机可执行文件（Windows、macOS、Linux），以及如何手动或通过自动化工具构建各硬件架构的 Docker 镜像。

---

## 环境准备

* **Go 语言环境**：Go 1.22 或更高版本（纯 Go 实现，无 CGO 依赖）；
* **FFmpeg**：原生单机运行时，需确保系统 PATH 中存在 `ffmpeg` 可执行文件（或放置在程序同级 `bin/` 目录下）；
* **Docker & Buildx**（仅构建镜像需要）：Docker 24.0+，并启用 Buildx 多架构构建插件。

---

## 1. 源码模块说明

项目所有交付物入口均位于 `cmd/` 目录：
* `cmd/slimbox`：服务端主程序（内嵌静态 Web 前端，集成 SQLite、转码调度器与 REST API）；
* `cmd/slimbox-tray`：桌面系统托盘常驻助手（提供状态监控、目录选择与浏览器唤起）；
* `cmd/slimbox-cli`：跨平台命令行客户端（支持远程批量推送、进度监控与成品回传）；
* `cmd/release`：多架构与多显卡 Docker 镜像矩阵全自动构建与发布工具。

---

## 2. 编译各平台单机版

### (1) Windows 版

#### 方式 A：使用自动化打包脚本（推荐）
项目提供了自动化打包脚本，会自动编译可执行文件并输出便携压缩包：
```powershell
# 在项目根目录下执行 PowerShell 脚本
powershell -ExecutionPolicy Bypass -File ./scripts/windows/build-zip.ps1
```
脚本执行完毕后，会在根目录或目标目录生成包含 `slimbox.exe`、`slimbox-tray.exe`、`slimbox-cli.exe` 的便携发行包。

#### 方式 B：手动执行 Go 编译命令
在项目根目录下分别执行以下命令：
```powershell
# 1. 编译服务端主程序
go build -ldflags="-s -w" -o bin/slimbox.exe ./cmd/slimbox

# 2. 编译桌面托盘助手 (加入 -H=windowsgui 隐藏控制台黑框)
go build -ldflags="-s -w -H=windowsgui" -o bin/slimbox-tray.exe ./cmd/slimbox-tray

# 3. 编译命令行客户端
go build -ldflags="-s -w" -o bin/slimbox-cli.exe ./cmd/slimbox-cli
```
> **提示**：运行 `slimbox.exe` 时，请确保系统已安装 FFmpeg，或在 `bin/` 目录下放置 `ffmpeg.exe`。

---

### (2) macOS 版

#### 方式 A：使用打包脚本
```bash
# 赋予脚本执行权限并打包 macOS 应用程序
chmod +x ./scripts/macos/build-dmg.sh
./scripts/macos/build-dmg.sh
```

#### 方式 B：手动执行 Go 编译命令
支持在任意平台进行交叉编译：

```bash
# 1. 编译 Apple Silicon 版 (M1 / M2 / M3 / M4)
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o bin/slimbox-darwin-arm64 ./cmd/slimbox
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o bin/slimbox-cli-darwin-arm64 ./cmd/slimbox-cli

# 2. 编译 Intel Mac 版 (x86_64)
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o bin/slimbox-darwin-amd64 ./cmd/slimbox
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o bin/slimbox-cli-darwin-amd64 ./cmd/slimbox-cli
```

---

### (3) Linux 单机版（不运行 Docker，直接系统级运行）

适用于在斐讯 N1、树莓派或 Linux 云服务器上直接作为独立进程或 systemd 服务运行：

```bash
# 1. 编译 Linux ARM64 版 (斐讯 N1、树莓派 3/4/5、RK3588 等)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/slimbox-linux-arm64 ./cmd/slimbox
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/slimbox-cli-linux-arm64 ./cmd/slimbox-cli

# 2. 编译 Linux x86_64 版 (普通 PC、云服务器)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/slimbox-linux-amd64 ./cmd/slimbox
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/slimbox-cli-linux-amd64 ./cmd/slimbox-cli
```

---

## 3. 构建 Docker 镜像

### 方式 A：使用内置全自动流水线脚本（推荐）

项目在 `cmd/release` 中提供了镜像构建工具，支持按需构建单变体或全量矩阵：

```bash
# 构建全量矩阵并推送 (arm64 + intel + amd + nvidia + latest)
go run ./cmd/release -flavor=matrix

# 仅构建专属变体
go run ./cmd/release -flavor=arm64   # ARM64 专版 (仅针对 linux/arm64)
go run ./cmd/release -flavor=intel   # Intel 核显专版 (仅针对 linux/amd64)
go run ./cmd/release -flavor=amd     # AMD 锐龙 APU / 独显专版 (仅针对 linux/amd64)
go run ./cmd/release -flavor=nvidia  # NVIDIA 独显专版 (基于 Dockerfile.nvidia)
go run ./cmd/release -flavor=latest  # 通用双驱动版 (含 latest 与 standard 标签)
```

---

### 方式 B：手动使用原生 Docker / Buildx 命令构建

根据实际硬件需求，直接在项目根目录下执行以下对应命令：

#### 1. 构建 ARM64 专版 (`slimbox:arm64`)
面向斐讯 N1、树莓派等 ARM 设备，剔除 x86 驱动，纯 CPU 软编轻量版：
```bash
docker buildx build \
  --platform linux/arm64 \
  --build-arg FLAVOR=arm64 \
  -t slimbox:arm64 \
  --load .
```

#### 2. 构建 Intel 核显加速版 (`slimbox:intel`)
面向 Intel 4~14 代核显（N100 等）及 Arc 独显，安装 Intel Media Driver 与 Libva：
```bash
docker buildx build \
  --platform linux/amd64 \
  --build-arg FLAVOR=intel \
  -t slimbox:intel \
  --load .
```

#### 3. 构建 AMD 核显与独显加速版 (`slimbox:amd`)
面向 AMD 锐龙 APU（如 5600G、7840HS）及 Radeon 独显，安装 Mesa VAAPI：
```bash
docker buildx build \
  --platform linux/amd64 \
  --build-arg FLAVOR=amd \
  -t slimbox:amd \
  --load .
```

#### 4. 构建 NVIDIA 独显加速版 (`slimbox:nvidia`)
面向搭载 NVIDIA 独立显卡的主机，采用 Debian 基础镜像并启用 NVENC：
```bash
docker build \
  -f Dockerfile.nvidia \
  -t slimbox:nvidia .
```

#### 5. 构建全功能通用开箱即用版 (`slimbox:latest`)
面向 x86_64 主机，同时安装 Intel 与 AMD 两套驱动：
```bash
docker buildx build \
  --platform linux/amd64 \
  --build-arg FLAVOR=all \
  -t slimbox:latest \
  -t slimbox:standard \
  --load .
```
