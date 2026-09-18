#!/bin/sh
# Boots one VM cold, waits for the agent, snapshots to build/snapshot/{vm.snap,vm.mem}.
# Layout must match what sandboxd's jailer chroot sees: drive at /rootfs.ext4, tap0, v.sock in cwd.
set -eu
cd "$(dirname "$0")"
rm -rf snapshot && mkdir -p snapshot
SOCK=/tmp/fc-snap.sock; rm -f $SOCK
LOG=/tmp/fc-snap.log
ip netns del snap 2>/dev/null || true
ip netns add snap
ip netns exec snap ip tuntap add tap0 mode tap
ip netns exec snap ip link set tap0 up
cp --reflink=auto --sparse=always rootfs.ext4 /rootfs.ext4
cd /tmp && rm -f v.sock

ip netns exec snap firecracker --api-sock $SOCK > $LOG 2>&1 &
FC=$!
sleep 0.2
api() { curl -sf --unix-socket $SOCK -X "$1" "http://localhost$2" -H 'Content-Type: application/json' -d "$3"; }
D=$(cd "$(dirname "$0")" && pwd)
api PUT /boot-source '{"kernel_image_path":"'$D'/vmlinux","boot_args":"console=ttyS0 reboot=k panic=1 pci=off ip=172.16.0.2::172.16.0.1:255.255.255.252::eth0:off init=/sbin/init"}'
api PUT /drives/rootfs '{"drive_id":"rootfs","path_on_host":"/rootfs.ext4","is_root_device":true,"is_read_only":false}'
api PUT /network-interfaces/eth0 '{"iface_id":"eth0","host_dev_name":"tap0","guest_mac":"06:00:AC:10:00:02"}'
api PUT /vsock '{"guest_cid":3,"uds_path":"v.sock"}'
api PUT /machine-config '{"vcpu_count":1,"mem_size_mib":512}'
api PUT /actions '{"action_type":"InstanceStart"}'
for i in $(seq 1 100); do grep -q "agent listening" $LOG && break; sleep 0.05; done
grep -q "agent listening" $LOG || { echo "agent never came up"; tail -20 $LOG; kill $FC; exit 1; }
sleep 0.3  # let the agent settle into accept()
api PATCH /vm '{"state":"Paused"}'
api PUT /snapshot/create '{"snapshot_type":"Full","snapshot_path":"'$D'/snapshot/vm.snap","mem_file_path":"'$D'/snapshot/vm.mem"}'
kill $FC; ip netns del snap; rm -f /rootfs.ext4
ls -la "$D/snapshot"
