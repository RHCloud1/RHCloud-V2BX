# 升级、迁移与诊断

## 常用命令

```bash
V2bX generate
V2bX check -c /etc/V2bX/config.json
V2bX doctor -c /etc/V2bX/config.json
V2bX doctor -c /etc/V2bX/config.json --network
V2bX restart
V2bX rollback
```

向导会按节点生成独立核心和出站文件。密码不回显，配置及备份权限为 0600，但 root 仍能读取，请保护服务器和备份。
同域名可以共用证书；不同域名分目录保存。HTTP 自动申请仍需要公网 TCP 80，DNS 模式则需要 DNS API 权限。

`doctor` 默认检查本地配置格式、证书身份和有效期，并列出 `ss -lntup`。
`--network` 额外调用配置中 Xboard 的节点接口，显示面板实际服务端口，解析证书域名，以及测试每个 sing-box SOCKS 出站的 TCP CONNECT。
探测有超时，不打印面板 token、代理密码和带凭据的请求 URL；不发送应用数据，不校验目标网站内容。
默认 CONNECT 目标为 `www.gstatic.com:443`，可通过 `--target example.com:443` 更改。
SOCKS5 code=2 表示网关按规则拒绝 CONNECT；不能单凭该错误确定限流、ASN 故障或账户状态。
本地有监听不代表公网安全组、防火墙或运营商路径可达，UDP 也不能用 TCP 检测代替。

## sing-box 旧配置迁移

1. 先备份 VPS 配置。上传本地新包和 SHA256，解压到独立目录，例如 `/root/v2bx-new`，不要覆盖当前服务。
2. 用新二进制预览每个 `OriginalPath` 文件的迁移。
3. 预览无误后显式写入；自动保留带时间戳的原文件备份。
4. 立即运行新安装器完成升级。不要在旧进程仍运行时改其他配置，避免旧版文件监视器重载新语法。

```bash
chmod +x /root/v2bx-new/V2bX
/root/v2bx-new/V2bX migrate-sing -c /etc/V2bX/sing_residential.json
/root/v2bx-new/V2bX migrate-sing -c /etc/V2bX/sing_residential.json --write
/root/v2bx-new/V2bX check -c /etc/V2bX/config.json
bash /root/v2bx-new/install.sh --archive /root/V2bX-linux-64.zip
```

迁移仅支持严格 JSON：常见旧 DNS address、出站 domain_strategy、命名 block 出站及对应路由。
block 路由改为 reject action；domain_strategy 改为指定 DNS 的 domain_resolver。
涉及 selector/block final、特殊 DNS、复杂旧策略或 JSON5 的情况会要求人工处理，不自动猜测。
迁移使用新内核的配置解析器验证格式，但不能证明路由意图、远程规则集或网络行为完全正确。
复杂配置应在独立测试 VPS 上迁移；不要直接对生产环境试错。

## 安装与回滚

安装顺序：下载 ZIP + SHA256、校验归档路径与完整性、检查配套脚本、运行新二进制的 version/check、备份旧程序、替换、检查服务状态。
脚本从同一 ZIP 安装，不再更新程序后单独下载 master 的管理脚本。
备份保存在 `/usr/local/.v2bx-backups/release.*`，指针为 `/usr/local/.v2bx-last-backup`。
健康检查仅确认进程保持 active，并不证明节点或面板业务可用；升级原本停用的服务时不会擅自启动。
安装过程不覆盖已有配置与证书，不清空防火墙；菜单 16 只尝试对 UFW/firewalld 放行指定端口，云安全组仍需另配。
安装器面向 apt/dnf/yum 的 systemd 发行版；Alpine/OpenRC 和容器服务管理不在本轮支持范围内。

启动失败时尝试恢复旧程序、单元、管理脚本及原运行状态，失败版本留在备份目录供排查。
`rollback` 也只恢复程序和配套脚本，不恢复 `/etc/V2bX` 的配置与证书。
如果已经迁移到新语法，回滚旧二进制前必须恢复对应配置备份，否则旧版可能不能启动。
首次安装没有旧程序可以回滚；系统依赖安装以及新增的辅助文件也不做系统级撤销。
异常断电或强制终止后可能残留安装锁，确认没有安装任务运行再处理 `/usr/local/.v2bx-install.lock`。

## 测试 VPS 验收清单

- Xboard 同步用户：添加、撤销、到期、恢复、限速和流量回报。
- HY2/AnyTLS 同时连接；分别测试 IPv4/IPv6，以及本地和云安全组规则。
- 住宅出口的实际 IP、拒绝行为、超时、UDP 限制，确认无直连回退。
- HTTP/DNS 证书申请、同/不同域名、续期失败后的重试与恢复。
- 服务重启、服务器重启、坏配置、坏包、升级失败和手动回滚。

本地代理通信测试已覆盖 sing-box 内的 HY2 + AnyTLS；没有用它代替独立 Hysteria 核心、真实面板或公网 ACME 验收。
用户列表真正变化仍会重建对应动态入站，不保证现有连接无中断；动态入站不支持作为 OriginalPath 静态入站的 detour 目标。
配置监视器会先检查新格式；完整配置重载仍非事务式切换，运行期失败恢复还需后续完善。升级或迁移时建议显式执行 check/restart。
面板下发单个节点的新端口绑定失败时，会尝试恢复该节点上一次可用的配置，并在下一次同步重试。这与整份主配置的事务式重载是两个不同功能。
模拟验收结果、流量重试限制和待办见 `simulation-20261108.md`。
