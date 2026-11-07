#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/../.."
work=$(mktemp -d /tmp/v2bx-wizard-test.XXXXXX)
trap 'rm -rf -- "$work"' EXIT
export V2BX_BINARY="$PWD/script/tests/fixtures/program.sh"
export V2BX_CONFIG="$work/config with spaces.json"
export V2BX_TEST_ARGS="$work/args"
chmod +x "$V2BX_BINARY"
source script/initconfig.sh
generate_config_file
grep -Fx configure "$work/args"
grep -Fx -- -c "$work/args"
grep -Fx "$V2BX_CONFIG" "$work/args"
if grep -q '^add_node_config()' script/V2bX.sh; then exit 1; fi
echo 'All configuration entry points use the embedded generator'
