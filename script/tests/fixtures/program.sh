#!/bin/bash
case "$1" in
    version) echo test-version ;;
    check) exit 0 ;;
    configure) printf '%s\n' "$@" > "$V2BX_TEST_ARGS" ;;
    *) exit 1 ;;
esac
