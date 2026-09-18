package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

// newOverlay creates a per-VM copy-on-write rootfs at <chroot>/rootfs.ext4.
// Uses reflink copy so it's instant and shares blocks with the base on btrfs/xfs.
// ponytail: falls back to a full copy on ext4 hosts. Swap for dm-snapshot or
// a virtio overlay drive if copy time or disk usage shows up in measurements.
func newOverlay(chroot string) (string, error) {
	dst := filepath.Join(chroot, "rootfs.ext4")
	if out, err := exec.Command("cp", "--reflink=auto", "--sparse=always", cfg.BaseRootfs, dst).CombinedOutput(); err != nil {
		return "", fmt.Errorf("overlay: %v: %s", err, out)
	}
	return dst, nil
}
