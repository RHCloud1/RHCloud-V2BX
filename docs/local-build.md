# 本地验证与打包

当前内核：sing-box 1.14.4、Hysteria 2.9.2、Xray 1.251208.0。
sing-box 升级参考 [官方版本](https://github.com/SagerNet/sing-box/releases/tag/v1.14.4) 和 [迁移说明](https://sing-box.sagernet.org/migration/)。
最低 Go 1.25.5，固定工具链 Go 1.25.11。没有修改系统 Go 安装或全局 go env。

Windows PowerShell 下，在仓库根目录执行：

```powershell
./script/test-linux.ps1
./script/build.ps1 -Version local-simulation.20261108 -Architectures amd64,arm64,s390x
```

测试脚本用本机 Go 交叉编译，在现有 WSL Ubuntu-22.04 中执行 Linux 测试，不安装系统级 Go。
安装器测试需要 WSL 中有 zip/unzip/zipinfo；若系统未安装，也可将可执行文件放入忽略目录 `dist/test-tools/runtime/usr/bin`。
测试包括 Go 测试、AnyTLS + HY2 本地真实代理请求、向导入口一致性和模拟 systemd 的安装/回滚测试。
不包含真实 Xboard、ACME 和供应商网络调用。Windows 原生服务端不是本轮交付目标。
模拟链路的具体覆盖与限制见 `simulation-20261108.md`。主配置完整重载回退和无断连更新仍未完成，不能把测试版称为生产验收通过。

产物位于 `dist/<版本>/`，包括各架构 ZIP 与 `.zip.sha256`。
包内包含二进制、示例 JSON、Geo 数据、文档和对应版本的安装/管理/配置入口脚本。
本地 Geo 数据沿用仓库快照，未声称是最新；GitHub 流水线会下载其自身的 Geo 数据。

## 测试本地包

源码未推送，GitHub 一键命令不会下载本轮改动。不要用示例配置覆盖生产配置。
先对 VPS 做快照或备份 `/etc/V2bX`、二进制和 systemd 单元。
将对应架构 ZIP 和同名 `.zip.sha256` 上传至 VPS 的独立目录，然后运行：

```bash
unzip -p V2bX-linux-64.zip install.sh > install-local.sh
bash install-local.sh --archive "$PWD/V2bX-linux-64.zip"
```

ARM64/s390x 使用各自文件名。新安装器会先校验包、检查现有配置，成功后替换程序，并保留完整旧程序备份。
旧 sing-box 配置如果包含移除的字段，会在停服务前拒绝升级；请先阅读 `operations.md` 的迁移流程。
新装后运行 `V2bX generate`，再 `V2bX check`、`V2bX start`。

## 发布

正式发布前仍需测试 VPS 验收。GitHub Release 发布事件触发构建；每个架构需要同时提供 ZIP 和 `.zip.sha256`。
旧 Release 如果没有校验文件，新安装器会拒绝安装，不降级为跳过校验。
SHA256 用于验证下载完整性，不替代对仓库和发布者的信任。
本次不执行提交、推送、打标签或发布。
