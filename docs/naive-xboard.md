# Naive 与 Xboard 部署说明

## 工作流程

1. 在 Xboard 新建 `naive` 类型节点，填写端口。Xboard 下发给服务端的字段只有端口、`tls` 和 `tls_settings`。
2. V2bX 通过 `/api/v1/server/UniProxy/config` 获取端口，通过 `/api/v1/server/UniProxy/user` 获取用户。
3. sing-box 创建 `naive` 入站，用户的 **UUID 同时作为用户名和密码**，和 Xboard 的 socks、http 节点一致。
4. Naive 客户端始终走 HTTPS，所以无论 Xboard 里的 `tls` 填 0 还是 1，V2bX 都按 TLS 提供服务，必须配置证书。
5. 用户变化时，V2bX 重建对应入站；只改限速或设备数时不重建，只更新限速器。

## V2bX 节点配置

一键脚本：运行 `V2bX generate`，核心选 `2.sing-box`，协议选 `8.Naive`，证书选 HTTP 或 DNS 自动申请。

手写配置示例见 `example/config_naive.json`，要点如下：

```json
{
  "Core": "sing",
  "NodeType": "naive",
  "ListenIP": "0.0.0.0",
  "NaiveQUIC": false,
  "CertConfig": {
    "CertMode": "http",
    "CertDomain": "naive.example.com",
    "CertFile": "/etc/V2bX/certs/{domain}/fullchain.cer",
    "KeyFile": "/etc/V2bX/certs/{domain}/cert.key",
    "Email": "admin@example.com"
  }
}
```

- `NaiveQUIC`：`false` 只提供 HTTP/2（TCP）；`true` 同时提供 HTTP/3，会额外占用同一端口的 UDP。如果同端口已有 HY2 等 UDP 服务，不要开启。
- 证书：NaiveProxy 官方客户端基于 Chromium，**不接受自签证书**，生产环境请用 `http`、`dns` 或已有的正式证书（`file`）。
- 只能用 sing-box 核心，Xray 和独立 HY2 核心都不支持 Naive。

## 客户端订阅：目前的缺口

截至 2026-11，Xboard master 的 11 种订阅格式（Clash、ClashMeta、sing-box、Shadowrocket、Surge、Loon、QuantumultX、Stash、Surfboard、General、Shadowsocks）**都不会输出 naive 节点**。服务端可以正常工作，但用户在订阅里看不到这个节点。

在 Xboard 支持之前，可以手动给客户端配置：

```text
naive+https://<用户UUID>:<用户UUID>@<节点域名>:<端口>#<名称>
```

NaiveProxy 官方客户端的 `config.json` 写法：

```json
{ "listen": "socks://127.0.0.1:1080", "proxy": "https://<UUID>:<UUID>@<节点域名>:<端口>" }
```

如果开启了 `NaiveQUIC`，把 `https://` 换成 `quic://` 即走 HTTP/3。

## 测试

- `core/sing/naive_test.go`：真实启动 naive 监听，验证面板 UUID 能通过鉴权、错误或已撤销的 UUID 被拒绝，以及 HTTP/3 开关。
- `api/panel/sync_test.go`：按 Xboard 实际下发的 naive JSON 解析节点（`tls: 0` 时仍按 TLS 处理）。
- `cmd/configure_test.go`：配置向导生成 Naive 节点，Xray 核心选 Naive 会被拒绝。

Naive 鉴权失败时不会返回 407，而是直接断开连接（防探测），排查时看服务端日志里的 `authorization failed`。

CONNECT 之后的数据转发使用 Naive 的 padding 格式，由 sing-box 自己负责并测试；这里没有用真实的 NaiveProxy 客户端做端到端测试，上线前需要在 VPS 上用客户端实测一次。
