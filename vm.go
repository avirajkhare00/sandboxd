package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/avirajkhare00/sandboxd/proto"
	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

const guestPort = 5000

type VM struct {
	ID      string
	machine *firecracker.Machine
	cancel  context.CancelFunc
	netns   string
	chroot  string // jailer chroot root; everything the VM touches lives here
}

func newID() string {
	b := make([]byte, 4) // 8 hex chars: keeps "sb-<id>h" under IFNAMSIZ (15)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// newVM restores one microVM from the golden snapshot (or cold boots if no snapshot).
// Paths inside the chroot are fixed (/rootfs.ext4, /vmlinux, tap0) so snapshot restore
// works: Firecracker requires the same drive paths and tap names as at snapshot time.
func newVM() (*VM, error) {
	id := newID()
	vm := &VM{ID: id, netns: "sb-" + id}
	vm.chroot = filepath.Join(cfg.StateDir, "firecracker", id, "root")
	if err := os.MkdirAll(vm.chroot, 0o755); err != nil {
		return nil, err
	}
	if err := setupVMNet(vm.netns); err != nil {
		vm.Destroy()
		return nil, err
	}
	overlay, err := newOverlay(vm.chroot)
	if err != nil {
		vm.Destroy()
		return nil, err
	}

	fcCfg := firecracker.Config{
		SocketPath:      "/run/firecracker.socket", // relative to chroot
		KernelImagePath: cfg.Kernel,
		KernelArgs:      "console=ttyS0 reboot=k panic=1 pci=off ip=172.16.0.2::172.16.0.1:255.255.255.252::eth0:off init=/sbin/init",
		Drives: []models.Drive{{
			DriveID:      firecracker.String("rootfs"),
			PathOnHost:   firecracker.String(overlay),
			IsRootDevice: firecracker.Bool(true),
			IsReadOnly:   firecracker.Bool(false),
		}},
		NetworkInterfaces: []firecracker.NetworkInterface{{
			StaticConfiguration: &firecracker.StaticNetworkConfiguration{HostDevName: "tap0", MacAddress: "06:00:AC:10:00:02"},
		}},
		VsockDevices: []firecracker.VsockDevice{{ID: "v", Path: "v.sock", CID: 3}},
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(cfg.VCPUs),
			MemSizeMib: firecracker.Int64(cfg.MemMiB),
		},
		NetNS: "/var/run/netns/" + vm.netns,
		JailerCfg: &firecracker.JailerConfig{
			ID:             id,
			UID:            firecracker.Int(1000),
			GID:            firecracker.Int(1000),
			NumaNode:       firecracker.Int(0),
			CgroupVersion:  "2",
			ExecFile:       cfg.Firecracker,
			JailerBinary:   cfg.Jailer,
			ChrootBaseDir:  cfg.StateDir,
			ChrootStrategy: firecracker.NewNaiveChrootStrategy(cfg.Kernel),
			Stdout:         os.Stdout,
			Stderr:         os.Stderr,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	vm.cancel = cancel
	var opts []firecracker.Opt
	if snap, mem := filepath.Join(cfg.SnapDir, "vm.snap"), filepath.Join(cfg.SnapDir, "vm.mem"); exists(snap) {
		// Snapshot files must be visible inside the chroot; hardlink them in.
		for _, f := range []string{snap, mem} {
			if err := os.Link(f, filepath.Join(vm.chroot, filepath.Base(f))); err != nil && !os.IsExist(err) {
				vm.Destroy()
				return nil, err
			}
		}
		opts = append(opts, firecracker.WithSnapshot("vm.mem", "vm.snap"))
	}
	m, err := firecracker.NewMachine(ctx, fcCfg, opts...)
	if err != nil {
		vm.Destroy()
		return nil, err
	}
	vm.machine = m
	if err := m.Start(ctx); err != nil {
		vm.Destroy()
		return nil, fmt.Errorf("start: %w", err)
	}
	return vm, nil
}

// Exec dials the guest agent over Firecracker's vsock UDS and runs one command.
func (vm *VM) Exec(ctx context.Context, req proto.Req) (*proto.Resp, error) {
	if vm.machine == nil {
		return nil, fmt.Errorf("vm not running")
	}
	d := net.Dialer{}
	c, err := d.DialContext(ctx, "unix", filepath.Join(vm.chroot, "v.sock"))
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if dl, ok := ctx.Deadline(); ok {
		c.SetDeadline(dl)
	} else {
		c.SetDeadline(time.Now().Add(2 * time.Minute))
	}
	// Firecracker host-side vsock handshake.
	fmt.Fprintf(c, "CONNECT %d\n", guestPort)
	r := bufio.NewReader(c)
	if line, err := r.ReadString('\n'); err != nil || len(line) < 2 || line[:2] != "OK" {
		return nil, fmt.Errorf("vsock connect: %q %v", line, err)
	}
	if err := proto.Write(c, req); err != nil {
		return nil, err
	}
	var resp proto.Resp
	if err := proto.Read(r, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (vm *VM) Destroy() error {
	if vm.machine != nil {
		vm.machine.StopVMM()
	}
	if vm.cancel != nil {
		vm.cancel()
	}
	teardownVMNet(vm.netns)
	return os.RemoveAll(filepath.Dir(vm.chroot))
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }
