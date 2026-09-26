# SlimBox 服务器部署信息

以下是 SlimBox 自托管服务器的连接、部署配置及全平台多架构规范信息：


## 1. 测试服务器（兼多架构构建机）
- **服务器 IP**：`192.168.7.186`
- **角色定位**：测试与验证环境，全架构 Docker 镜像构建机
- **SSH 账户**：`docker`
- **SSH 密码**：`docker`
- **Root 密码**：`demon3434`
- **Docker 持久化根目录**：`/opt/docker/slimbox`
- **源码与构建目录**：`/opt/docker/slimbox/src`
- **统一安全网关访问**：`http://192.168.7.186:8083`
- **核心流程**：在 186 上运行单测、集成测试与功能验证，并通过 Docker Buildx 构建 `linux/amd64`、`linux/arm64` 多架构镜像，一键 push 到阿里云镜像仓库（ACR）。