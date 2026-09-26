# SlimBox Domain Context

SlimBox 是一个运行在低功耗 ARM 设备（如斐讯 N1、树莓派等）Docker 容器内的视频压缩微服务与命令行工具，旨在以极小文件体积压制电影剧集，提供 Web 界面与批量 CLI 客户端。

## Language

**Task (任务)**:
视频压缩处理的基本单元，包含用户上传的原视频、选定的压缩参数/档位、转码状态及最终生成的压缩文件。
_Avoid_: Job, process, conversion

**Transcoding Profile (压缩档位)**:
预设的视频压缩参数组合（如 x264 快速档位、x265 极致压缩档位），提供给用户一键选用。
_Avoid_: Preset, template

**Media Probe (媒体源参数探测)**:
通过探针对上传的源视频文件进行流分析提取出的元数据，包含视频编解码格式、分辨率、帧率、码率及音频编码与轨道数，用于在界面展示和指导压缩参数配置。
_Avoid_: Inspection, video-info

**Execution Queue (转码执行队列)**:
管理视频转码执行的排队通道，严格执行单任务串行调度，确保同一时间仅运行一个 FFmpeg 转码进程以保护硬件安全。
_Avoid_: Thread pool, batch runner

**Resolution Preset Matrix (全分辨率预设矩阵)**:
针对初级用户预设的常用电影压制参数集合，覆盖从 360p 到 4K 的各主流分辨率；默认编码器选用 H.265 以达成极限小文件体积，每个档位对应 CLI 命令行枚举值，支持用户自由调整并持久化保存自身预设及恢复出厂默认值。
_Avoid_: Auto-profile, smart-preset

**Custom Preset Store (用户自定义档位库)**:
持久化保存用户微调后的个性化预设参数，并与系统出厂默认值并存，支持单键还原。
_Avoid_: User config, settings JSON

**Task Abort Mechanism (任务实时中止机制)**:
允许用户在 Web UI 或 CLI 随时终止正在运行的 FFmpeg 转码进程，安全回收系统 CPU/内存资源并清理未完成的残片文件。
_Avoid_: Kill switch, force quit

**Optional Auth Gateway (可选鉴权网关)**:
通过环境变量或配置启用的内网/外网访问凭证机制，支持局域网免密直接使用，或配合 API Key / 访问密码保护接口。
_Avoid_: Hardcoded auth, single sign-on

**Advanced Parameter Canvas (高级自定义参数画布)**:
针对高级用户的调参面板，支持对视频编码、CRF、预设速度、音频编码码率及自定义参数的精细控制。
_Avoid_: Settings modal

**All-track Audio Preservation (全音轨保留策略)**:
在预设档位下对源视频内包含的所有音频轨道（原声、国配、评论等）予以全量保留，并统一压缩为低码率 AAC 格式，平衡体积与多语言完整性。
_Avoid_: Primary track only, audio strip

**Soft Subtitle Passthrough (软字幕直通封装)**:
在压缩封装过程中无损复制源视频所有内嵌字幕轨道（copy），不向视频画面烧录硬字幕，保留多语言选择灵活性且零画质损耗。
_Avoid_: Hardcode burn-in, drop subtitles

**Partial Artifact Garbage Collection (半成品残片回收)**:
任务因转码报错失败或被用户手动中止时，系统立即自动扫描并物理删除所产生的半成品文件碎片，避免低效挤占 USB 磁盘空间。
_Avoid_: Orphan cleanup, temp purge

**Storage Lifecycle Policy (存储生命周期策略)**:
关于原片暂存与转码成品在 USB 外接盘上的留存规则，支持转码完成即删源文件、下载完成即清成品等独立开关配置。
_Avoid_: Cache eviction, temp-cleanup


**CLI State Manifest (CLI 状态清单)**:
命令行客户端在外部设备上记录批量任务状态的文件，用于支持网络断开或中断退出时的断点跳过与续批。
_Avoid_: Log file, history

**CLI Client (命令行客户端)**:
运行在外部操作端（如 PC）的非图形化工具，通过网络 API 与 SlimBox 服务通信，执行批量视频的逐个上传、排队监控与结果下载。
_Avoid_: Script, daemon


