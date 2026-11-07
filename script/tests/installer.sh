#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/../.."
work=$(mktemp -d /tmp/v2bx-installer-test.XXXXXX)
trap 'rm -rf -- "$work"' EXIT
export V2BX_ROOT="$work/root"
export V2BX_SYSTEMCTL="$PWD/script/tests/fixtures/systemctl.sh"
export V2BX_HEALTH_DELAY=0
chmod +x "$V2BX_SYSTEMCTL"
source script/install.sh
mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$(dirname "$SERVICE_FILE")" "$(dirname "$MENU_FILE")" "$work/package"
cp script/tests/fixtures/program.sh "$INSTALL_DIR/V2bX"
printf '# OLD_VERSION\n' >> "$INSTALL_DIR/V2bX"
printf 'old menu\n' > "$MENU_FILE"
printf 'old unit\n' > "$SERVICE_FILE"
printf '{"keep":"config"}\n' > "$CONFIG_DIR/config.json"
cp "$CONFIG_DIR/config.json" "$work/config.before"
touch "$ROOT/active"
for file in install.sh initconfig.sh V2bX.sh V2bX.service; do cp "script/$file" "$work/package/$file"; done
cp script/tests/fixtures/program.sh "$work/package/V2bX"
(cd "$work/package"; zip -q "$work/good.zip" ./*)
sha256sum "$work/good.zip" > "$work/good.zip.sha256"

# Corrupt checksum: nothing, including the running service, may change.
printf '%064d  good.zip\n' 0 > "$work/bad.sha256"
if install_archive "$work/good.zip" "$work/bad.sha256"; then exit 1; fi
grep -q OLD_VERSION "$INSTALL_DIR/V2bX"
test -f "$ROOT/active"

# Start failure: program, wrapper, unit and running state are restored.
printf '# FAIL_START\n' >> "$work/package/V2bX"
(cd "$work/package"; zip -q "$work/fail.zip" ./*)
sha256sum "$work/fail.zip" > "$work/fail.zip.sha256"
if install_archive "$work/fail.zip" "$work/fail.zip.sha256"; then exit 1; fi
grep -q OLD_VERSION "$INSTALL_DIR/V2bX"
grep -q 'old menu' "$MENU_FILE"
grep -q 'old unit' "$SERVICE_FILE"
test -f "$ROOT/active"
cmp "$work/config.before" "$CONFIG_DIR/config.json"

# Successful update and explicit rollback.
install_archive "$work/good.zip" "$work/good.zip.sha256"
if grep -q OLD_VERSION "$INSTALL_DIR/V2bX"; then exit 1; fi
read -r previous < "$(dirname "$INSTALL_DIR")/.v2bx-last-backup"
restore_backup "$previous" true
grep -q OLD_VERSION "$INSTALL_DIR/V2bX"
test -f "$ROOT/active"
cmp "$work/config.before" "$CONFIG_DIR/config.json"
# Older releases only ship an openssl .dgst list.
{ echo "MD5= 00"; echo "SHA2-256= $(sha256sum "$work/good.zip" | cut -d' ' -f1)"; } > "$work/good.zip.dgst"
checksum_from_dgst "$work/good.zip.dgst" "$work/from-dgst.sha256"
install_archive "$work/good.zip" "$work/from-dgst.sha256"
printf 'MD5= 00\n' > "$work/empty.dgst"
if checksum_from_dgst "$work/empty.dgst" "$work/empty.sha256" 2>/dev/null; then exit 1; fi

# Old-format package without management scripts is refused without touching the install.
mkdir -p "$work/legacy"
cp script/tests/fixtures/program.sh "$work/legacy/V2bX"
(cd "$work/legacy"; zip -q "$work/legacy.zip" ./*)
sha256sum "$work/legacy.zip" > "$work/legacy.zip.sha256"
cp "$INSTALL_DIR/V2bX" "$work/program.before"
if install_archive "$work/legacy.zip" "$work/legacy.zip.sha256" 2>/dev/null; then exit 1; fi
cmp "$work/program.before" "$INSTALL_DIR/V2bX"

# Backups are pruned to the newest V2BX_KEEP_BACKUPS, never the rollback target.
export V2BX_KEEP_BACKUPS=2
for _ in 1 2 3 4; do sleep 1; install_archive "$work/good.zip" "$work/good.zip.sha256" >/dev/null; done
count=$(ls -1d "$BACKUP_ROOT"/release.* | wc -l)
[[ "$count" -eq 2 ]] || { echo "expected 2 backups, found $count" >&2; exit 1; }
read -r previous < "$(dirname "$INSTALL_DIR")/.v2bx-last-backup"
test -d "$previous/program"
echo 'Installer checksum/dgst fallback/legacy refusal/failure rollback/success/manual rollback/backup pruning tests passed'
