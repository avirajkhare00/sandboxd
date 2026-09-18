#!/bin/sh
# Boots one VM cold, waits for the agent, snapshots to build/snapshot/{vm.snap,vm.mem}.
# Must be run with the same tap name (tap0) and drive path (/rootfs.ext4) sandboxd uses.
set -eu
cd "$(dirname "$0")"
mkdir -p snapshot
SOCK=/tmp/fc-snap.sock; rm -f $SOCK
ip netns add snap 2>/dev/null || true
ip netns exec snap ip tuntap add tap0 mode tap 2>/dev/null || true
ip netns exec snap ip link set tap0 up
cp --reflink=auto rootfs.ext4 /rootfs.ext4

ip netns exec snap firecracker --api-sock $SOCK &
sleep 0.2
api() { curl -s --unix-socket $SOCK -X "$1" "http://localhost$2" -H 'Content-Type: application/json' -d "$3"; }
api PUT /boot-source '{"kernel_image_path":"'$PWD'/vmlinux","boot_args":"console=ttyS0 reboot=k panic=1 pci=off ip=172.16.0.2::172.16.0.1:255.255.255.252::eth0:off init=/sbin/init"}'
api PUT /drives/rootfs '{"drive_id":"rootfs","path_on_host":"/rootfs.ext4","is_root_device":true,"is_read_only":false}'
api PUT /network-interfaces/eth0 '{"iface_id":"eth0","host_dev_name":"tap0","guest_mac":"06:00:AC:10:00:02"}'
api PUT /vsock '{"vsock_id":"v","guest_cid":3,"uds_path":"v.sock"}'
api PUT /machine-config '{"vcpu_count":1,"mem_size_mib":512}'
api PUT /actions '{"action_type":"InstanceStart"}'
sleep 3   # TODO: poll vsock until agent answers instead of sleeping
api PATCH /vm '{"state":"Paused"}'
api PUT /snapshot/create '{"snapshot_type":"Full","snapshot_path":"'$PWD'/snapshot/vm.snap","mem_file_path":"'$PWD'/snapshot/vm.mem"}'
kill %1
echo "snapshot written to $PWD/snapshot"
