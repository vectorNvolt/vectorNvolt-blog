#!/usr/bin/env bash
# Installed as /usr/local/libexec/agent-host-isolate, run by agent-host-isolate.service before
# tmp.mount and docker.service. WSL injects these mounts into every distro regardless of wsl.conf;
# unmount them lazily in this distro's mount namespace. Fails if any is still mounted afterwards.
# It must run before tmp.mount: once /tmp is a tmpfs, umount cannot reach /tmp/.X11-unix by path.
set -euo pipefail

mounts=(/mnt/wslg /tmp/.X11-unix /mnt/wsl /usr/lib/wsl)

for m in "${mounts[@]}"; do
    # Not mounted is fine. Several nested mounts may exist, so repeat until none is left.
    while mountpoint -q "$m" 2>/dev/null; do
        # Keep umount's error in the journal; the check below decides whether this is fatal.
        umount -l "$m" || break
    done
done

failed=0
for m in "${mounts[@]}"; do
    if mountpoint -q "$m" 2>/dev/null; then
        echo "agent-host-isolate: $m is still mounted" >&2
        failed=1
    fi
done
[ "$failed" -eq 0 ] || exit 1

# /dev/dxg is a kernel device node on the devtmpfs /dev, shared with other distros in the same VM, so it must not be
# deleted or chmod'ed. Bind /dev/null over it in this distro only: make /dev private first so the bind cannot propagate.
if [ -e /dev/dxg ]; then
    # Already masked by an earlier run: /dev/null is char 1:3, the real device is a:7d.
    if [ "$(stat -c %t:%T /dev/dxg)" != "1:3" ]; then
        mount --make-private /dev
        mount --bind /dev/null /dev/dxg
    fi
    if [ "$(stat -c %t:%T /dev/dxg)" != "1:3" ]; then
        echo "agent-host-isolate: /dev/dxg is still the GPU device" >&2
        exit 1
    fi
fi

# Defense in depth behind [interop] enabled=false: unregister the interop entries from binfmt_misc.
for e in WSLInterop WSLInterop-late; do
    if [ -e "/proc/sys/fs/binfmt_misc/$e" ]; then
        echo -1 >"/proc/sys/fs/binfmt_misc/$e"
    fi
    if [ -e "/proc/sys/fs/binfmt_misc/$e" ]; then
        echo "agent-host-isolate: $e is still registered" >&2
        exit 1
    fi
done
