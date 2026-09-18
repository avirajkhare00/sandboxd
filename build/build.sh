#!/bin/sh
# Builds the guest kernel + base rootfs + guest agent. Run on the KVM host.
set -eu
cd "$(dirname "$0")"
ARCH=$(uname -m)
FC_VER=v1.13.0

# 1. Firecracker + jailer
if [ ! -x /usr/local/bin/firecracker ]; then
  curl -sL "https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VER}/firecracker-${FC_VER}-${ARCH}.tgz" | tar xz
  sudo install "release-${FC_VER}-${ARCH}/firecracker-${FC_VER}-${ARCH}" /usr/local/bin/firecracker
  sudo install "release-${FC_VER}-${ARCH}/jailer-${FC_VER}-${ARCH}" /usr/local/bin/jailer
fi

# 2. Kernel (prebuilt CI kernel from the Firecracker project)
[ -f vmlinux ] || curl -sL -o vmlinux "https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/v1.13/${ARCH}/vmlinux-6.1.bin"

# 3. Guest agent
(cd .. && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/agent ./guest)

# 4. Base rootfs: debian + python + node + git, agent as init
[ -f rootfs.ext4 ] || {
  truncate -s 2G rootfs.ext4
  mkfs.ext4 -q rootfs.ext4
  mkdir -p mnt && sudo mount rootfs.ext4 mnt
  cid=$(sudo docker run -d debian:bookworm-slim sh -c 'apt-get update -qq && apt-get install -y -qq --no-install-recommends python3 python3-pip nodejs npm git ca-certificates curl iproute2 >/dev/null')
  sudo docker wait "$cid" >/dev/null
  sudo docker export "$cid" | sudo tar -x -C mnt
  sudo docker rm "$cid" >/dev/null
  sudo cp agent mnt/usr/local/bin/agent
  sudo mkdir -p mnt/work
  sudo tee mnt/sbin/init >/dev/null <<'INIT'
#!/bin/sh
mount -t proc proc /proc; mount -t sysfs sys /sys; mount -t devtmpfs dev /dev
exec /usr/local/bin/agent
INIT
  sudo chmod +x mnt/sbin/init
  sudo umount mnt && rmdir mnt
}
echo "done: vmlinux rootfs.ext4 agent"
