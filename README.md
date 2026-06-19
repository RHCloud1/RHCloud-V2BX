# RHCloud-V2BX

基于 V2bX 继续维护的多内核 Xboard 节点服务端。

当前版本集成：

- sing-box 1.13.13
- Xray 1.251208.0
- Hysteria 2.9.2
- Xboard AnyTLS 节点支持

支持 Vmess、Vless、Trojan、Shadowsocks、Hysteria、Hysteria2 和 AnyTLS 等协议。

## 一键安装

```bash
wget -O install.sh "https://raw.githubusercontent.com/RHCloud1/RHCloud-V2BX/master/script/install.sh?$(date +%s)" && bash install.sh
```

安装指定版本：

```bash
wget -O install.sh "https://raw.githubusercontent.com/RHCloud1/RHCloud-V2BX/master/script/install.sh?$(date +%s)" && bash install.sh v0.0.0-anytls.20260611
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

## Release 文件

一键安装脚本会根据服务器架构下载：

- `V2bX-linux-64.zip`
- `V2bX-linux-arm64-v8a.zip`
- `V2bX-linux-s390x.zip`

## 本地构建

需要 Go 1.25 或更高版本：

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
