# HY2、AnyTLS 与住宅出口

## 各字段负责什么

| 配置位置 | 字段 | 作用 |
| --- | --- | --- |
| Xboard | 节点地址 | 客户端实际连接的域名或 IP |
| Xboard | 连接端口 / 服务端口 | 客户端连接端口 / V2bX 实际监听端口，无端口转发时应相同 |
| Xboard | SNI | 必须由服务端证书覆盖 |
| V2bX | ListenIP | 绑定哪一个本机地址；不是 DNS 或证书域名 |
| V2bX | CertDomain | 自动申请证书的身份，也是已有证书 SAN 的本地校验依据 |
| V2bX | CoreName | 将节点分配到指定内核实例，该实例的出站规则影响其全部节点 |

HY2 使用 UDP，AnyTLS 使用 TCP。同一数字端口也可分别绑定 TCP 和 UDP；例如 HY2 UDP 8443、AnyTLS TCP 443 更便于排查。
端口从 Xboard 获取，不应在本地主配置里另造 `Port` 字段。

## 示例

`example/config_hy2_anytls.json` 使用独立 HY2 核心直连、sing-box AnyTLS 核心走住宅出口。
将 `example/sing_residential.json` 放到 `/etc/V2bX/sing_residential.json`，替换面板和住宅代理凭据，再按实际情况修改 NodeID、域名和监听地址。

示例中的 HY2 监听 `::`，AnyTLS 监听 `0.0.0.0`。`::` 在部分 Linux 系统上也接受 IPv4；若要求仅 IPv6，请绑定该服务器的具体 IPv6 地址。
入站使用 IPv6 不代表出口使用 IPv6，住宅出口由 SOCKS 服务决定。

证书文件按域名存放在 `/etc/V2bX/certs/<域名>/`。
两个节点可以使用同一域名和证书；不同域名应分别保存。已有多 SAN 证书可通过 `CertMode: file` 供多个域名复用。
历史的 `/etc/V2bX/fullchain.cer` 和 `/etc/V2bX/cert.key` 路径仍支持，只要证书匹配对应域名。
代码不会把错误域名的已有证书自动覆盖，避免中断仍在使用它的节点。

HTTP 自动申请需要证书域名解析到服务器，且 TCP 80 能被证书服务访问。
面板 SNI、客户端实际 SNI 和证书 SAN 要一致；切换 A/AAAA 记录不会自动更改证书。

## 住宅出口的限制

示例仅启用 SOCKS TCP，未匹配的请求全部走住宅出口，没有自动直连回退。
只有确认供应商支持 SOCKS5 UDP ASSOCIATE 并实测可用后，才去掉 `network: tcp`。
阻断规则用 `action: reject`，避免新增已经弃用的 `block` 出站。

`socks5: request rejected, code=2` 表示 SOCKS 网关拒绝请求，`code=4` 表示网关报告目标不可达。
这类日志表明请求已经进入住宅出站流程；不能仅凭错误码断言供应商限流或具体运营商故障。

## 部署前与部署后检查

```bash
V2bX check -c /etc/V2bX/config.json
V2bX doctor -c /etc/V2bX/config.json
V2bX doctor -c /etc/V2bX/config.json --network
ss -lntp | grep ':443'
ss -lunp | grep ':8443'
journalctl -u V2bX --since '5 minutes ago' --no-pager
```

`check` 检查主配置、内核选择、重复节点、监听 IP 和已有证书的密钥/SAN；管理模式的过期证书允许进入启动续期流程。
它会使用当前内核解析 sing-box OriginalPath 的字段格式，但不验证其完整运行语义、远程规则集可用性等。
它不访问 Xboard，不读取面板实际端口，不测试公网 DNS、防火墙或 SOCKS 账号。
不存在的自动申请证书允许通过，启动时再申请；`file` 模式的证书必须已经存在且有效。

sing-box 当前通过重建对应入站应用真实的用户增删。重复添加、空更新和删除不存在的用户不会重建；更新失败会尝试恢复之前的用户。
此机制不是无断连热更新。内核升级后需要继续评估公开热更新接口。

`generate` 现由 Go 程序统一实现，为每个节点生成独立 CoreName；sing-box 节点可分别选择直连或住宅出口。
住宅模式只启用 SOCKS TCP，不作直连回退；AnyTLS 客户端的 UDP 请求不代表供应商支持 SOCKS UDP。
`doctor --network` 才会联系面板、解析 DNS 和测试住宅 SOCKS CONNECT；其默认模式不做这些探测。
