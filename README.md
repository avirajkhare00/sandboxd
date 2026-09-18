# sandboxd

Firecracker microVM sandbox service for agent harnesses. One binary, warm pool,
snapshot restore, per-VM netns with egress allowlist, vsock exec.

Needs a Linux host with `/dev/kvm`, `ip`, `nft`, `docker`. Does not run on macOS.

```sh
./build/build.sh                 # firecracker, kernel, rootfs, guest agent
sudo ./build/snapshot.sh         # golden snapshot for fast restore
go build -o sandboxd . && sudo ./sandboxd -pool 4 -egress 151.101.0.0/16,104.16.0.0/12

curl -XPOST localhost:8080/sandboxes                       # {"id":"..."}
curl -XPOST localhost:8080/sandboxes/$ID/exec -d '{"cmd":["python3","-c","print(1+1)"]}'
curl -XPUT  localhost:8080/sandboxes/$ID/files/a.py --data-binary @a.py
curl -XDELETE localhost:8080/sandboxes/$ID
```

Numbers to collect for the blog: cold boot vs restore (ms), RSS per idle VM,
VMs per box, exec p50/p99 under load, overlay disk growth per hour.
