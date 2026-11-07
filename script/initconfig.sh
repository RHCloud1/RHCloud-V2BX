#!/bin/bash
# All entry points use the generator embedded in the matching V2bX binary.
generate_config_file() {
    local binary="${V2BX_BINARY:-/usr/local/V2bX/V2bX}"
    "$binary" configure -c "${V2BX_CONFIG:-/etc/V2bX/config.json}"
}
if [[ "${BASH_SOURCE[0]}" = "$0" ]]; then generate_config_file "$@"; fi
