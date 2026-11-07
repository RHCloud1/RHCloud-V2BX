# AnyTLS 与 Xboard 部署说明

## 工作流程

1. Xboard 保存 AnyTLS 节点参数和用户 UUID。
2. V2bX 通过 `/api/v1/server/UniProxy/config` 获取端口、TLS 和 `padding_scheme`。
3. V2bX 通过 `/api/v1/server/UniProxy/user` 获取用户。
4. sing-box 创建 AnyTLS 入站，并把用户 UUID 同时作为用户名标识和密码。
5. 用户变化时，V2bX 重建对应入站，使新用户列表立即生效。

## V2bX 节点配置

```json
{
  "Core": "sing",
  "ApiHost": "https://panel.example.com",
  "ApiKey": "replace-with-xboard-api-key",
  "NodeID": 1,
  "NodeType": "anytls",
  "ListenIP": "::",
  "CertConfig": {
    "CertMode": "http",
    "CertDomain": "node.example.com",
    "CertFile": "/etc/V2bX/certs/{domain}/fullchain.cer",
    "KeyFile": "/etc/V2bX/certs/{domain}/cert.key",
    "Email": "admin@example.com"
  }
}
```

AnyTLS 必须启用 TLS。`CertMode` 可使用 `http`、`dns`、`file` 或 `self`，不能使用 `none`。
`padding_scheme` 不需要写入本地配置，它由 Xboard 节点配置下发。当前实现兼容 JSON 字符串数组和多行字符串。

## GitHub Release

安装脚本按服务器架构下载以下固定文件名：

- `V2bX-linux-64.zip`
- `V2bX-linux-arm64-v8a.zip`
- `V2bX-linux-s390x.zip`

创建 GitHub Release 后，把对应 ZIP 上传为 Release Assets。`install.sh` 会读取
`RHCloud1/RHCloud-V2BX` 的 latest release，并下载匹配当前架构的文件。

AnyTLS 不需要安装独立内核或额外系统软件。它已经编译进 V2bX 使用的 sing-box 库。
安装脚本现有的 `curl`、`wget`、`unzip`、`socat`、证书和 systemd 相关依赖可以继续使用。

## 当前版本

- Go: 1.25
- sing-box: 1.14.4
- Hysteria: 2.9.2
- Xray: 1.251208.0

Xray 固定在该版本，是为了与 sing-box 和 Hysteria 共用的 `qpack 0.6` 依赖保持兼容。
