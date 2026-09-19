#!/bin/sh
# One-shot install on a fresh Debian/Ubuntu host with /dev/kvm. Run as root.
#   curl -sL https://raw.githubusercontent.com/avirajkhare00/sandboxd/main/install.sh | sh
set -eu
[ -e /dev/kvm ] || { echo "no /dev/kvm: need bare metal or a VM with nested virtualization"; exit 1; }
export PATH=$PATH:/usr/local/go/bin:/usr/sbin
apt-get update -qq
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nftables iptables docker.io git curl xfsprogs e2fsprogs >/dev/null
command -v go >/dev/null || { curl -sL https://go.dev/dl/go1.26.5.linux-amd64.tar.gz | tar -C /usr/local -xz; }

[ -d sandboxd ] || git clone -q https://github.com/avirajkhare00/sandboxd
cd sandboxd
go build -o /usr/local/bin/sandboxd . && go build -o /usr/local/bin/sandboxd-mcp ./cmd/sandboxd-mcp
sh build/build.sh

# reflink-capable state dir: without this every sandbox costs a full rootfs copy (see NOTES.md)
if ! mountpoint -q /srv/sbx; then
  mkdir -p /srv/sbx
  [ -f /srv/sbx.img ] || { truncate -s 60G /srv/sbx.img && mkfs.xfs -q -m reflink=1 /srv/sbx.img; }
  mount -o loop /srv/sbx.img /srv/sbx
fi
cp build/rootfs.ext4 build/vmlinux /srv/sbx/
sh build/snapshot.sh
mkdir -p /srv/sbx/snapshot && cp build/snapshot/vm.snap build/snapshot/vm.mem /srv/sbx/snapshot/

install -m644 build/sandboxd.service /etc/systemd/system/
systemctl daemon-reload && systemctl enable --now sandboxd
sleep 3 && systemctl --no-pager status sandboxd | head -5
echo; echo "try:  curl -XPOST localhost:8080/sandboxes"
echo "egress allowlist: edit EGRESS in /etc/default/sandboxd, then systemctl restart sandboxd"
