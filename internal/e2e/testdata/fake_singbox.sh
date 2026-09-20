#!/bin/sh
# fake sing-box used by hermetic tests (accepts the real CLI surface we rely on)
if [ "$1" = "version" ]; then
    echo "fake-sing-box version 1.0.0-test"
    exit 0
fi
if [ "$1" != "check" ]; then
    echo "usage: fake-sing-box check -c FILE | version" >&2
    exit 2
fi
shift
if [ "$1" = "-c" ]; then
    shift
fi
file="$1"
if [ -z "$file" ] || [ ! -f "$file" ]; then
    echo "fake-sing-box: config file not found" >&2
    exit 1
fi
if grep -q '__fake_invalid__' "$file"; then
    echo "fake-sing-box: decode config: invalid config" >&2
    exit 1
fi
exit 0
