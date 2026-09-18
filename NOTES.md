# Field notes (raw material for the blog)

Host: GCE n2-standard-4 (4 vCPU Cascade Lake @ 2.8GHz, 16GB), asia-south1-a, Debian 13,
nested virtualization ON. Numbers below are therefore *nested KVM*; expect bare metal to be faster.
Guest: 1 vCPU, 512MB, Debian bookworm rootfs (python3, node 18, git, curl), Firecracker v1.13.0, kernel 6.1.141.

## Measured 2026-09-18

| metric | value |
|---|---|
| cold boot -> agent answers over vsock | ~1.40s (1.31-1.43s over 8 boots) |
| snapshot restore -> agent answers | p50 119ms, min 81ms, max 162ms (n=25, pool refills running concurrently) |
| idle firecracker RSS after restore | ~29MB (mem file is mmapped; pages fault in lazily) |
| disk per live VM (ext4 host, no reflink) | ~680MB actual / 2GB apparent per overlay copy |
| snapshot size | vm.mem 512MB, vm.snap 15KB |

## Things that bit (chronological)

1. `cp -a / /out` inside a container to build a rootfs copies /out into itself. Use `docker export`.
2. Firecracker CI kernel URL: the bucket has `vmlinux-6.1.141`, not `vmlinux-6.1.bin`. The 314-byte "kernel" was an S3 XML error.
3. nftables: `fwd`, `nat`, `snat` are keywords. Chain names collided. Prefix them.
4. Linux IFNAMSIZ is 15. `sb-<12 hex>h` = 16 chars -> "Attribute failed policy validation". Shortened IDs to 8 hex.
5. Debian 13 is cgroup v2 only; the jailer defaults to v1 -> `CgroupHierarchyMissing`. Set `CgroupVersion: "2"`.
6. The Go SDK's jailer hardlinks drives into the chroot itself. Pre-placing the overlay there -> `file exists`. Put it one dir up.
7. Jailer drops firecracker to uid 1000; a root-owned overlay -> `Permission denied` opening the block device. chown it.
8. Go's `net.FileListener` doesn't know AF_VSOCK (`getsockname: address family not supported`). Accept by hand with x/sys/unix.
9. init starts with an empty environment. No PATH -> `exec: "python3": executable file not found`. Set PATH in the agent.
10. My forward chain with `policy drop` killed Docker networking on the host (apt hung inside containers). Filter `iifname sb0` only.
11. Docker installs `FORWARD` policy DROP in its own iptables table. Any table's drop wins -> allowed egress silently blackholed. Add `-i/-o sb0 ACCEPT`.
12. SDK v1.0.0 snapshot mode + jailer don't compose: it stats chroot-relative paths on the host, re-adds the vsock device that the snapshot already restores, and never links the rootfs into the chroot. Three handler-list edits fix it.
13. `pkill sandboxd` under sudo matches the `sudo pkill sandboxd` wrapper and kills that first. Use `pkill -x` or pids.
14. Restored guests have the clock frozen at snapshot time (`date` says 18:55 forever). Needs a clock fix on first exec.

## Open

- Guest clock after restore (ptp_kvm or agent sets time from host on first request).
- Guest DNS (no resolv.conf; allowlist by IP only for now).
- Reflink-capable host filesystem (xfs/btrfs) to get overlay cost near zero.
- Load test: N concurrent exec p50/p99, VMs per box until OOM.
- Bare metal rerun for un-nested numbers.
