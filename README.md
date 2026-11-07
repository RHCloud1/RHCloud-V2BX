# RHCloud-V2BX

基于 V2bX 继续维护的多内核 Xboard 节点服务端。

当前版本集成：

- sing-box 1.14.4
- Xray 1.251208.0
- Hysteria 2.9.2
- Xboard AnyTLS 节点支持
- Xboard Naive 节点支持（服务端；Xboard 订阅暂不输出 naive，见 [docs/naive-xboard.md](docs/naive-xboard.md)）

支持 Vmess、Vless、Trojan、Shadowsocks、Hysteria、Hysteria2、AnyTLS 和 Naive 等协议。

> 当前安装脚本只支持新格式的 Release（压缩包内含管理脚本，附带 `.sha256` 或 `.dgst`）。
> 2026-06 的 `v0.0.0-anytls.20260611` 是旧格式，新脚本会拒绝安装；推送新脚本时需同时发布新 Release。

## 一键安装

```bash
wget -O install.sh "https://raw.githubusercontent.com/RHCloud1/RHCloud-V2BX/master/script/install.sh?$(date +%s)" && bash install.sh
```

安装指定版本（将 `YOUR_RELEASE_TAG` 替换为包含 ZIP 和 `.sha256` 的 Release 标签）：

```bash
wget -O install.sh "https://raw.githubusercontent.com/RHCloud1/RHCloud-V2BX/master/script/install.sh?$(date +%s)" && bash install.sh YOUR_RELEASE_TAG
```

安装完成后运行：

```bash
V2bX
```

生成节点配置：

```bash
V2bX generate
```

配置 AnyTLS 时选择：

1. 核心选择 `singbox`
2. 协议选择 `AnyTLS`
3. 配置可用的 TLS 证书模式

AnyTLS 必须启用 TLS。可使用 HTTP/DNS 自动申请、已有证书或自签证书。

## 仓库地址

- 项目主页：https://github.com/RHCloud1/RHCloud-V2BX
- Releases：https://github.com/RHCloud1/RHCloud-V2BX/releases
- Issues：https://github.com/RHCloud1/RHCloud-V2BX/issues
- AnyTLS 部署说明：[docs/anytls-xboard.md](docs/anytls-xboard.md)
- 开发提纲与完成情况：[docs/roadmap.md](docs/roadmap.md)
- HY2 + AnyTLS 与住宅出口：[docs/multi-node.md](docs/multi-node.md)
- 本地验证与打包：[docs/local-build.md](docs/local-build.md)
- 升级、迁移、诊断与回滚：[docs/operations.md](docs/operations.md)
- 模拟验收与修复记录：[docs/simulation-20261108.md](docs/simulation-20261108.md)

## Release 文件

一键安装脚本会根据服务器架构下载：

- `V2bX-linux-64.zip`
- `V2bX-linux-arm64-v8a.zip`
- `V2bX-linux-s390x.zip`

每个 ZIP 必须同时上传同名 `.zip.sha256`；新安装器不会跳过校验。
安装器只在新包通过校验和配置检查后替换程序，失败时尝试恢复旧程序，保留 `/etc/V2bX`。
源码改动尚未发布时，以上远程命令不会获取本地开发版本，测试方法见本地打包文档。

## 本地构建

最低 Go 1.25.5，仓库固定并使用 Go 1.25.11 工具链；本轮目标平台为 Linux：

```bash
go build -v -o ./V2bX \
  -tags "sing xray hysteria2 with_quic with_grpc with_utls with_wireguard with_acme with_gvisor" \
  -trimpath -ldflags "-s -w -buildid="
```

## 来源与致谢

本项目基于 V2bX、Xray、sing-box 和 Hysteria 等开源项目继续维护，保留原项目许可证。

- https://github.com/wyx2685/V2bX
- https://github.com/XTLS/Xray-core
- https://github.com/SagerNet/sing-box
- https://github.com/apernet/hysteria
