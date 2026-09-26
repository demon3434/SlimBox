# SlimBox ⚡

> 运行在斐讯 N1、树莓派等低功耗 ARM 设备上的极致轻量视频压缩微服务与命令行工具。

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Architecture](https://img.shields.io/badge/Arch-linux%2Farm64%20%7C%20amd64-blue)](https://github.com)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker)](https://docker.com)

平时下载的电影电视剧动辄数 GB 到数十 GB，对于仓储和移动观影极具压力。**SlimBox** 专为“只要看得清就行、文件尽量最小”的诉求设计，充分利用斐讯 N1、树莓派等设备常年通电在线的特性，提供 Web 界面与自动化 CLI 客户端。

---

## 🌟 核心特性与架构原则

1. **为受限 ARM 硬件量身定制 (ADR-0001, ADR-0006)**：
   - 后端使用 Go 静态编译，运行时内存驻留仅 **15~30MB**，无额外解释器开销；
   - 采用纯 Go 实现的 SQLite（`modernc.org/sqlite`，零 CGO 依赖），ARM64 编译部署极度轻巧；
   - 依赖本地外挂物理 USB 移动硬盘（`/data` 挂载点），无需 N1 复杂挂载网络文件系统。
2. **严格单任务串行调度 (ADR-0002)**：
   - 文件上传与下载并发解耦，但底层 FFmpeg 转码执行器严格保证 **1/1 单任务独占 FIFO 调度**，杜绝 2GB 内存 OOM 与高温死机。
3. **全分辨率预设矩阵 (360p ~ 4K) (ADR-0003, ADR-0007)**：
   - 覆盖 360p、480p、720p、1080p、2K、4K 常用电影压制配置；
   - **默认采用 H.265 (HEVC)** 追求极致文件体积缩减，亦可一键切换为 H.264；
   - **中立自主选择**：系统绝不帮用户强选默认档位，必须由使用者显式确认；
   - 支持将自定义参数保存为用户档位预设，或一键恢复出厂默认值。
4. **全音轨保留与软字幕直通封装 (ADR-0010)**：
   - 预设档位下**保留源视频包含的所有音频轨道**（原声、国配、导评等），并统一压为紧凑高保真的 AAC 格式；
   - 所有内嵌软字幕采用 **`-c:s copy` 直通复制**，零画质折损、零多余算力消耗，播放器可任意开关。
5. **随时中止与半成品残片即时回收 (ADR-0008, ADR-0011)**：
   - Web 仪表盘与 CLI 均支持**随时强行终止当前运行的任务**；
   - 无论是手动中止还是源文件损坏导致转码失败，系统立即自动执行**残片清理（物理删除 `.part` 文件）**，不占磁盘空间，并自动调度执行下一个任务。
6. **非图形化 CLI 批处理与断点跳过 (ADR-0005)**：
   - 提供独立可执行文件 `slimbox-cli`，支持通过指定目录或视频清单文件，逐个向 N1 自动上传、轮询监控、下载成品回本地；
   - 本地自动维护 `.slimbox-state.json`，遇到网络闪断或重启可**平滑断点续批**。

---

## 🚀 快速部署 (Docker / N1 树莓派)

### 1. 准备 USB 硬盘挂载点
在您的斐讯 N1（如 Armbian / OpenWrt 系统）或树莓派上，假设 USB 移动硬盘挂载在 `/mnt/usb_disk`：

```bash
mkdir -p /mnt/usb_disk
```

### 2. 使用 Docker Compose 一键启动

在项目目录下执行：

```bash
docker compose up -d
```

或使用原生 `docker run`：

```bash
docker run -d \
  --name slimbox \
  --restart unless-stopped \
  -p 8080:8080 \
  -v /mnt/usb_disk:/data \
  slimbox:latest
```

启动完成后，打开浏览器访问：`http://<设备IP>:8080` 即可进入高质感深色 Web 仪表盘。

---

## 💻 CLI 批处理客户端使用指南

`slimbox-cli` 是跨平台单一命令行工具，可在您的 PC 或 Mac 上直接运行，将本地大量剧集/电影自动推送给 N1 转码并取回：

```bash
# 1. 批量处理一个本地目录（将 D:\Movies 中的视频逐个推送给 N1 压制为 720p H.265 并取回至 D:\Compressed）
slimbox-cli batch -s "http://192.168.1.100:8080" \
                  -d "D:\Movies" \
                  -o "D:\Compressed" \
                  --profile 720p

# 2. 指定视频文件清单文本（逐行读取）
slimbox-cli batch -s "http://192.168.1.100:8080" \
                  -l "series_list.txt" \
                  -o "D:\Compressed" \
                  --profile 1080p

# 3. 查询当前 N1 转码执行器状态与队列
slimbox-cli status -s "http://192.168.1.100:8080"

# 4. 远程强行中止某个正在转码的任务
slimbox-cli abort -s "http://192.168.1.100:8080" --task-id <TASK_ID>
```

---

## 📁 目录结构

```
SlimBox/
├── cmd/
│   ├── slimbox/          # 后端服务主入口（内嵌静态 Web 资源）
│   └── slimbox-cli/      # 命令行批处理客户端
├── internal/
│   ├── api/              # RESTful API 路由、中间件与硬件探测器
│   ├── domain/           # 核心领域模型与 360p~4K 预设矩阵
│   ├── engine/           # FFprobe 分析器、FFmpeg 封装器与残片清理器
│   ├── repository/       # 纯 Go SQLite 持久化层
│   └── scheduler/        # 严格串行单任务 FIFO 队列与存储生命周期管理
├── web/                  # 现代深色仪表盘（HTML / CSS / Vanilla JS）
├── docs/
│   └── adr/              # ADR 架构决策记录 (0001-0011)
├── Dockerfile            # 多架构 (ARM64/AMD64) 多阶段构建规范
├── docker-compose.yml    # 容器编排部署文件
└── CONTEXT.md            # 领域统一通用语言词汇表
```

---

## 📜 许可协议
MIT License.
