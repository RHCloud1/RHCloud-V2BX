#!/bin/bash
set -euo pipefail

REPO="RHCloud1/RHCloud-V2BX"
ROOT="${V2BX_ROOT:-}"
INSTALL_DIR="${ROOT}/usr/local/V2bX"
CONFIG_DIR="${ROOT}/etc/V2bX"
SERVICE_FILE="${ROOT}/etc/systemd/system/V2bX.service"
MENU_FILE="${ROOT}/usr/bin/V2bX"
BACKUP_ROOT="${ROOT}/usr/local/.v2bx-backups"
SYSTEMCTL="${V2BX_SYSTEMCTL:-systemctl}"

download() {
    curl --fail --location --retry 3 --connect-timeout 15 --max-time 300 "$1" -o "$2"
}

# Releases before .sha256 was added only ship an openssl .dgst list
# ("SHA2-256= <hex>"); turn that line into the sha256sum format we verify.
checksum_from_dgst() {
    local dgst=$1 out=$2 hash
    hash=$(awk '$1 == "SHA2-256=" || $1 == "SHA256=" { print $2; exit }' "$dgst")
    [[ "$hash" =~ ^[[:xdigit:]]{64}$ ]] || { echo "校验文件里没有 SHA256 值" >&2; return 1; }
    printf '%s  package.zip\n' "$hash" > "$out"
}

# Prefer <zip>.sha256; fall back to <zip>.dgst for older releases.
fetch_checksum() {
    local url=$1 out=$2
    if download "$url.sha256" "$out" 2>/dev/null; then return 0; fi
    echo "该版本没有 .sha256，改用 .dgst 中的 SHA256" >&2
    download "$url.dgst" "$out.dgst" || { echo "该版本没有可用的校验文件，已停止安装" >&2; return 1; }
    checksum_from_dgst "$out.dgst" "$out"
}

# Keep the newest backups (default 3) plus the one manual rollback points to.
prune_backups() {
    local keep=${V2BX_KEEP_BACKUPS:-3} last="" dir count=0
    [[ "$keep" =~ ^[0-9]+$ && "$keep" -ge 1 ]] || keep=3
    if [[ -f "$(dirname "$INSTALL_DIR")/.v2bx-last-backup" ]]; then
        read -r last < "$(dirname "$INSTALL_DIR")/.v2bx-last-backup" || last=""
    fi
    while IFS= read -r dir; do
        [[ -n "$dir" ]] || continue
        count=$((count + 1))
        if (( count > keep )) && [[ "$dir" != "$last" && "$dir" = "$BACKUP_ROOT"/release.* ]]; then
            rm -rf -- "$dir"
        fi
    done < <(ls -1dt -- "$BACKUP_ROOT"/release.* 2>/dev/null || true)
}

validate_archive() {
    local archive=$1 checksum=$2 expected actual name
    read -r expected _ < "$checksum"
    [[ "$expected" =~ ^[[:xdigit:]]{64}$ ]] || { echo "无效的 SHA256 文件" >&2; return 1; }
    actual=$(sha256sum "$archive")
    [[ "${actual%% *}" = "${expected,,}" ]] || { echo "安装包 SHA256 不匹配，未修改现有安装" >&2; return 1; }
    # Refuse path traversal, symbolic links and archives with unexpected roots.
    while IFS= read -r name; do
        [[ "$name" != /* && "$name" != *\\* && "/$name/" != */../* ]] || return 1
    done < <(unzip -Z1 "$archive")
    if zipinfo -l "$archive" | grep '^l' >/dev/null; then
        echo "安装包含符号链接，已拒绝" >&2; return 1
    fi
    unzip -tq "$archive" >/dev/null
}

restore_backup() {
    local backup=$1 was_running=$2
    backup=$(realpath "$backup") || return 1
    [[ "$backup" = "$BACKUP_ROOT"/release.* && -d "$backup/program" ]] || {
        echo "没有可恢复的旧版本" >&2; return 1;
    }
    "$SYSTEMCTL" stop V2bX || true
    if [[ -e "$INSTALL_DIR" ]]; then
        mv -- "$INSTALL_DIR" "$backup/failed-program" || return 1
    fi
    mv -- "$backup/program" "$INSTALL_DIR" || return 1
    for item in service menu; do
        local target=$SERVICE_FILE
        [[ "$item" = menu ]] && target=$MENU_FILE
        if [[ -e "$backup/$item" ]]; then
            cp -p -- "$backup/$item" "$target" || return 1
        elif [[ -f "$backup/new-$item" ]]; then
            rm -f -- "$target" || return 1
        fi
    done
    "$SYSTEMCTL" daemon-reload || return 1
    if [[ "$was_running" = true ]]; then
        "$SYSTEMCTL" start V2bX || return 1
        "$SYSTEMCTL" is-active --quiet V2bX || return 1
    fi
    echo "已恢复旧版本，失败版本保留在 $backup/failed-program"
}

install_archive() (
    local archive=$1 checksum=$2 stage work backup="" was_running=false
    local promoted=false committed=false locked=false had_program=false
    local parent
    parent=$(dirname "$INSTALL_DIR")
    mkdir -p -- "$parent" "$BACKUP_ROOT" "$(dirname "$MENU_FILE")" "$(dirname "$SERVICE_FILE")" || return 1
    mkdir -- "$parent/.v2bx-install.lock" || { echo "已有安装任务运行中" >&2; return 1; }
    locked=true
    work=$(mktemp -d "$parent/.v2bx-install.XXXXXX") || { rmdir "$parent/.v2bx-install.lock"; return 1; }
    cleanup() {
        local status=$?
        trap - EXIT
        if [[ "$promoted" = true && "$committed" = false ]]; then
            if [[ "$had_program" = true ]]; then
                restore_backup "$backup" "$was_running" || echo "自动回滚失败，备份位于 $backup" >&2
            else
                "$SYSTEMCTL" stop V2bX || true
                [[ ! -d "$INSTALL_DIR" ]] || mv -- "$INSTALL_DIR" "$backup/failed-program"
                for item in service menu; do
                    local target=$SERVICE_FILE
                    [[ "$item" = menu ]] && target=$MENU_FILE
                    if [[ -e "$backup/$item" ]]; then cp -p -- "$backup/$item" "$target"
                    else rm -f -- "$target"; fi
                done
                "$SYSTEMCTL" daemon-reload || true
            fi
        fi
        [[ "$work" != "$parent"/.v2bx-install.* ]] || rm -rf -- "$work"
        [[ "$locked" != true ]] || rmdir -- "$parent/.v2bx-install.lock"
        exit "$status"
    }
    trap cleanup EXIT
    validate_archive "$archive" "$checksum" || return 1
    stage="$work/program"
    mkdir "$stage" || return 1
    unzip -q "$archive" -d "$stage" || return 1
    for file in V2bX V2bX.sh initconfig.sh install.sh V2bX.service; do
        [[ -f "$stage/$file" ]] || {
            echo "安装包缺少 $file：这是旧格式的 Release（不含管理脚本），本安装器只支持新版 Release，请改装最新版本" >&2
            return 1
        }
    done
    for file in V2bX.sh initconfig.sh install.sh; do bash -n "$stage/$file" || return 1; done
    chmod 755 "$stage/V2bX" "$stage/"*.sh || return 1
    "$stage/V2bX" version >/dev/null || return 1
    if [[ -f "$CONFIG_DIR/config.json" ]]; then
        "$stage/V2bX" check -c "$CONFIG_DIR/config.json" || {
            echo "新版本不接受现有配置，未停止旧服务" >&2; return 1;
        }
    fi
    backup=$(mktemp -d "$BACKUP_ROOT/release.XXXXXX") || return 1
    if [[ -e "$SERVICE_FILE" ]]; then cp -p -- "$SERVICE_FILE" "$backup/service" || return 1
    else touch "$backup/new-service" || return 1; fi
    if [[ -e "$MENU_FILE" ]]; then cp -p -- "$MENU_FILE" "$backup/menu" || return 1
    else touch "$backup/new-menu" || return 1; fi
    if "$SYSTEMCTL" is-active --quiet V2bX; then was_running=true; fi
    if [[ -d "$INSTALL_DIR" ]]; then
        had_program=true
        "$SYSTEMCTL" stop V2bX || return 1
        if ! mv -- "$INSTALL_DIR" "$backup/program"; then
            [[ "$was_running" != true ]] || "$SYSTEMCTL" start V2bX
            return 1
        fi
    fi
    promoted=true
    mv -- "$stage" "$INSTALL_DIR" || return 1
    install -m 644 "$INSTALL_DIR/V2bX.service" "$SERVICE_FILE" || return 1
    install -m 755 "$INSTALL_DIR/V2bX.sh" "$MENU_FILE" || return 1
    mkdir -p -- "$CONFIG_DIR" || return 1
    for file in geoip.dat geosite.dat dns.json route.json custom_outbound.json custom_inbound.json; do
        if [[ -f "$INSTALL_DIR/$file" && ! -e "$CONFIG_DIR/$file" ]]; then
            cp -- "$INSTALL_DIR/$file" "$CONFIG_DIR/$file" || return 1
        fi
    done
    "$SYSTEMCTL" daemon-reload || return 1
    if [[ "$was_running" = true ]]; then
        "$SYSTEMCTL" start V2bX || return 1
        for _ in 1 2 3; do
            sleep "${V2BX_HEALTH_DELAY:-1}"
            "$SYSTEMCTL" is-active --quiet V2bX || { echo "新服务启动失败，正在回滚" >&2; return 1; }
        done
    fi
    if [[ ! -e "${ROOT}/usr/bin/v2bx" ]]; then ln -s V2bX "${ROOT}/usr/bin/v2bx"; fi
    if [[ "$had_program" = true ]]; then printf '%s\n' "$backup" > "$parent/.v2bx-last-backup"; fi
    committed=true
    prune_backups || echo "清理旧备份失败，可手动删除 $BACKUP_ROOT 下较早的目录" >&2
    echo "安装成功。配置和证书已保留。旧版本备份：$backup"
)

install_base() {
    if command -v apt-get >/dev/null; then
        apt-get update
        apt-get install -y curl unzip ca-certificates
    elif command -v dnf >/dev/null; then
        dnf install -y curl unzip ca-certificates
    elif command -v yum >/dev/null; then
        yum install -y curl unzip ca-certificates
    else echo "需要 Debian/Ubuntu 或支持 dnf/yum 的 Linux" >&2; return 1
    fi
}

main() {
    [[ $EUID -eq 0 ]] || { echo "请使用 root 运行" >&2; return 1; }
    if [[ "${1:-}" = --rollback ]]; then
        local previous running=false
        mkdir -- "$(dirname "$INSTALL_DIR")/.v2bx-install.lock" || { echo "已有安装或回滚任务运行中" >&2; return 1; }
        trap 'rmdir -- "$(dirname "$INSTALL_DIR")/.v2bx-install.lock"' EXIT
        read -r previous < "$(dirname "$INSTALL_DIR")/.v2bx-last-backup"
        "$SYSTEMCTL" is-active --quiet V2bX && running=true
        restore_backup "$previous" "$running"
        rmdir -- "$(dirname "$INSTALL_DIR")/.v2bx-install.lock"
        trap - EXIT
        return
    fi
    install_base
    if [[ "${1:-}" = --archive ]]; then
        [[ $# -eq 2 ]] || { echo "用法: bash install.sh --archive /path/package.zip" >&2; return 1; }
        local archive checksum
        archive=$(realpath "$2")
        checksum="$archive.sha256"
        if [[ ! -f "$checksum" && -f "$archive.dgst" ]]; then
            checksum=$(mktemp)
            checksum_from_dgst "$archive.dgst" "$checksum"
        fi
        install_archive "$archive" "$checksum"
    else
        local version="${1:-}" arch work url
        case "$(uname -m)" in
            x86_64|amd64) arch=64 ;;
            aarch64|arm64) arch=arm64-v8a ;;
            s390x) arch=s390x ;;
            *) echo "不支持的架构" >&2; return 1 ;;
        esac
        if [[ -z "$version" ]]; then
            url=$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest")
            version="${url##*/}"
        fi
        [[ "$version" =~ ^[a-zA-Z0-9._+-]+$ ]] || { echo "无效版本号" >&2; return 1; }
        work=$(mktemp -d)
        trap 'rm -rf -- "$work"' EXIT
        url="https://github.com/$REPO/releases/download/$version/V2bX-linux-$arch.zip"
        download "$url" "$work/package.zip"
        fetch_checksum "$url" "$work/package.zip.sha256"
        install_archive "$work/package.zip" "$work/package.zip.sha256"
        rm -rf -- "$work"
        trap - EXIT
    fi
    if [[ ! -f "$CONFIG_DIR/config.json" ]]; then
        local answer
        read -rp "是否现在生成节点配置？(y/n): " answer || answer=n
        if [[ "$answer" =~ ^[Yy]$ ]]; then
            "$INSTALL_DIR/V2bX" configure -c "$CONFIG_DIR/config.json"
            [[ ! -f "$CONFIG_DIR/config.json" ]] || "$SYSTEMCTL" enable --now V2bX
        else echo "安装完成，运行 V2bX generate 配置节点后再启动。"; fi
    fi
}

if [[ "${BASH_SOURCE[0]}" = "$0" ]]; then main "$@"; fi
