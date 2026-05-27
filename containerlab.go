package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const containerlabImage = "ghcr.io/zinrai/docker-ubuntu-bird3:ubuntu-resolute"

func generateContainerlab(t *Topology, outputDir string) error {
	// build interface index per node first (needed for exec addr assignment)
	ifaceIndex := make(map[string]int)
	// nodeIfaceAddrs maps nodeName -> list of (ifaceName, addr)
	nodeIfaceAddrs := make(map[string][]struct{ iface, addr string })
	for _, link := range t.Links {
		ifaceIndex[link.ANode]++
		aIface := fmt.Sprintf("eth%d", ifaceIndex[link.ANode])
		ifaceIndex[link.BNode]++
		bIface := fmt.Sprintf("eth%d", ifaceIndex[link.BNode])

		nodeIfaceAddrs[link.ANode] = append(nodeIfaceAddrs[link.ANode], struct{ iface, addr string }{aIface, link.AAddr})
		nodeIfaceAddrs[link.BNode] = append(nodeIfaceAddrs[link.BNode], struct{ iface, addr string }{bIface, link.BAddr})
	}

	var sb strings.Builder

	sb.WriteString("name: backbonelab\n\n")
	sb.WriteString("topology:\n")
	sb.WriteString("  nodes:\n")

	for _, node := range sortedNodes(t) {
		sb.WriteString(fmt.Sprintf("    %s:\n", node.Name))
		sb.WriteString("      kind: linux\n")
		sb.WriteString(fmt.Sprintf("      image: %s\n", containerlabImage))
		sb.WriteString("      binds:\n")
		sb.WriteString(fmt.Sprintf("        - %s/bird.conf:/etc/bird/bird.conf\n", node.Name))
		sb.WriteString("      exec:\n")

		// loopback address
		sb.WriteString(fmt.Sprintf("        - ip addr add %s dev lo\n", node.Loopback))

		// link interface addresses
		for _, ia := range nodeIfaceAddrs[node.Name] {
			sb.WriteString(fmt.Sprintf("        - ip link set %s up\n", ia.iface))
			sb.WriteString(fmt.Sprintf("        - ip addr add %s dev %s\n", ia.addr, ia.iface))
		}

		// IPv4 forwarding
		sb.WriteString("        - sysctl -w net.ipv4.ip_forward=1\n")

		// start bird in foreground
		sb.WriteString("        - mkdir -p /run/bird\n")
		sb.WriteString("        - bird -c /etc/bird/bird.conf\n")
	}

	sb.WriteString("\n  links:\n")

	// reset ifaceIndex for link generation
	ifaceIndex = make(map[string]int)
	for _, link := range t.Links {
		ifaceIndex[link.ANode]++
		aIface := fmt.Sprintf("eth%d", ifaceIndex[link.ANode])
		ifaceIndex[link.BNode]++
		bIface := fmt.Sprintf("eth%d", ifaceIndex[link.BNode])
		sb.WriteString(fmt.Sprintf("    - endpoints: [\"%s:%s\", \"%s:%s\"]\n",
			link.ANode, aIface, link.BNode, bIface))
	}

	path := filepath.Join(outputDir, ".clab.yaml")
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("failed to write .clab.yaml: %w", err)
	}

	return nil
}
