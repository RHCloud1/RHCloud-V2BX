#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/../.."
archive=$(realpath "${1:?Usage: package.sh /path/V2bX-linux-64.zip}")
work=$(mktemp -d /tmp/v2bx-package-test.XXXXXX)
trap 'rm -rf -- "$work"' EXIT
export V2BX_ROOT="$work/root"
export V2BX_SYSTEMCTL="$PWD/script/tests/fixtures/systemctl.sh"
chmod +x "$V2BX_SYSTEMCTL"
source script/install.sh
mkdir -p "$CONFIG_DIR"
printf '%s\n' '{"Cores":[{"Type":"sing"}],"Nodes":[{"Core":"sing","ApiHost":"https://panel.example","ApiKey":"dummy-key","NodeID":1,"NodeType":"vless"}]}' > "$CONFIG_DIR/config.json"
cp "$CONFIG_DIR/config.json" "$work/config.before"
install_archive "$archive" "$archive.sha256"
"$INSTALL_DIR/V2bX" version
"$INSTALL_DIR/V2bX" check -c "$CONFIG_DIR/config.json"
cmp "$CONFIG_DIR/config.json" "$work/config.before"
cmp "$INSTALL_DIR/V2bX.sh" "$MENU_FILE"
"$INSTALL_DIR/V2bX" doctor -c "$CONFIG_DIR/config.json"
echo 'Real Linux package install/check/local doctor smoke tests passed'
