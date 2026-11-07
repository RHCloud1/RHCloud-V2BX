#!/bin/bash
set -eu
case "$1" in
    is-active) test -f "$V2BX_ROOT/active" ;;
    stop) rm -f "$V2BX_ROOT/active" ;;
    start)
        if grep -q FAIL_START "$V2BX_ROOT/usr/local/V2bX/V2bX"; then exit 1; fi
        touch "$V2BX_ROOT/active"
        ;;
    daemon-reload|enable) ;;
    *) exit 1 ;;
esac
