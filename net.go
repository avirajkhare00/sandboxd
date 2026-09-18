package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// Network model: each VM gets its own netns with tap0 (172.16.0.1/30) so every
// snapshot sees the identical device name. A veth pair links the netns to the
// host bridge sb0; nftables on the host NATs and enforces the egress allowlist.
//
// ponytail: shells out to ip/nft. Replace with netlink + nftables libs if
// sandbox churn makes fork/exec cost visible.

func sh(cmd string) error {
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", cmd, err, out)
	}
	return nil
}

func setupHostNet(allow []string) error {
	set := "{ 0.0.0.0/32 }" // deny-all placeholder
	if len(allow) > 0 && allow[0] != "" {
		set = "{ " + strings.Join(allow, ", ") + " }"
	}
	cmds := []string{
		"ip link add sb0 type bridge 2>/dev/null || true",
		"ip addr replace 10.200.0.1/16 dev sb0",
		"ip link set sb0 up",
		"sysctl -qw net.ipv4.ip_forward=1",
		"nft add table inet sandboxd 2>/dev/null || true",
		"nft flush table inet sandboxd",
		// policy accept so docker0 and friends keep working; only sb0 traffic is filtered.
		"nft add chain inet sandboxd sb_egress '{ type filter hook forward priority 0; policy accept; }'",
		"nft add rule inet sandboxd sb_egress ct state established,related accept",
		"nft add rule inet sandboxd sb_egress iifname sb0 ip daddr " + set + " accept",
		"nft add rule inet sandboxd sb_egress iifname sb0 drop",
		// docker sets FORWARD policy DROP in its own table; any table's drop wins, so punch sb0 through.
		"iptables -C FORWARD -i sb0 -j ACCEPT 2>/dev/null || iptables -I FORWARD -i sb0 -j ACCEPT",
		"iptables -C FORWARD -o sb0 -j ACCEPT 2>/dev/null || iptables -I FORWARD -o sb0 -j ACCEPT",
		"nft add chain inet sandboxd sb_nat '{ type nat hook postrouting priority 100; }'",
		"nft add rule inet sandboxd sb_nat ip saddr 10.200.0.0/16 oifname != sb0 masquerade",
	}
	for _, c := range cmds {
		if err := sh(c); err != nil {
			return err
		}
	}
	return nil
}

// setupVMNet: netns + tap0 inside it + veth to host bridge. Guest IP is always
// 172.16.0.2 inside its netns; the netns SNATs to a unique 10.200.x.y on the bridge.
func setupVMNet(ns string) error {
	host := ns + "h"
	ip := nsIP(ns)
	cmds := []string{
		"ip netns add " + ns,
		"ip netns exec " + ns + " ip tuntap add tap0 mode tap",
		"ip netns exec " + ns + " ip addr add 172.16.0.1/30 dev tap0",
		"ip netns exec " + ns + " ip link set tap0 up",
		"ip link add " + host + " type veth peer name veth0 netns " + ns,
		"ip link set " + host + " master sb0 up",
		"ip netns exec " + ns + " ip addr add " + ip + "/16 dev veth0",
		"ip netns exec " + ns + " ip link set veth0 up",
		"ip netns exec " + ns + " ip link set lo up",
		"ip netns exec " + ns + " ip route add default via 10.200.0.1",
		"ip netns exec " + ns + " sysctl -qw net.ipv4.ip_forward=1",
		"ip netns exec " + ns + " nft add table ip nat",
		"ip netns exec " + ns + " nft add chain ip nat post '{ type nat hook postrouting priority 100; }'",
		"ip netns exec " + ns + " nft add rule ip nat post oifname veth0 masquerade",
	}
	for _, c := range cmds {
		if err := sh(c); err != nil {
			teardownVMNet(ns)
			return err
		}
	}
	return nil
}

func teardownVMNet(ns string) {
	sh("ip link del " + ns + "h 2>/dev/null")
	sh("ip netns del " + ns + " 2>/dev/null")
}

// nsIP derives a stable 10.200.x.y from the netns name.
// ponytail: hash-based, collisions possible past ~thousands of live VMs; use an allocator then.
func nsIP(ns string) string {
	var h uint32
	for _, c := range ns {
		h = h*31 + uint32(c)
	}
	x, y := 1+(h>>8)%254, 1+h%254
	return fmt.Sprintf("10.200.%d.%d", x, y)
}
