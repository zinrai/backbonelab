package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Build-time variables, populated by goreleaser via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	var (
		outputDir   string
		showVersion bool
	)
	flag.StringVar(&outputDir, "output", "./output", "output directory")
	flag.BoolVar(&showVersion, "version", false, "show version and exit")
	flag.Parse()

	if showVersion {
		fmt.Printf("backbonelab %s (commit %s, built %s)\n", version, commit, date)
		return
	}

	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: backbonelab [options] <topology.yaml>\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	configPath := flag.Arg(0)

	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	topo, err := buildTopology(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building topology: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// generate containerlab topology
	if err := generateContainerlab(topo, outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating containerlab topology: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("generated: %s\n", filepath.Join(outputDir, ".clab.yaml"))

	// generate bird configs for each node
	for _, node := range sortedNodes(topo) {
		nodeDir := filepath.Join(outputDir, node.Name)
		if err := os.MkdirAll(nodeDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating node directory %s: %v\n", nodeDir, err)
			os.Exit(1)
		}

		var content string
		switch node.Role {
		case RoleCore:
			content = generateBirdCore(node, topo)
		case RoleRR:
			cluster := clusterForNode(topo, node.Name)
			content = generateBirdRR(node, topo, cluster)
		case RoleEdge:
			peers := externalPeersForEdge(cfg, topo, node.Name)
			content = generateBirdEdge(node, topo, peers)
		case RoleExternal:
			content = generateBirdExternal(node, topo)
		}

		outPath := filepath.Join(nodeDir, "bird.conf")
		if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", outPath, err)
			os.Exit(1)
		}
		fmt.Printf("generated: %s\n", outPath)
	}
}

func sortedNodes(t *Topology) []*Node {
	names := make([]string, 0, len(t.Nodes))
	for name := range t.Nodes {
		names = append(names, name)
	}
	sort.Strings(names)
	nodes := make([]*Node, 0, len(names))
	for _, name := range names {
		nodes = append(nodes, t.Nodes[name])
	}
	return nodes
}

func clusterForNode(t *Topology, nodeName string) *RRCluster {
	for i := range t.RRClusters {
		for _, m := range t.RRClusters[i].Members {
			if m == nodeName {
				return &t.RRClusters[i]
			}
		}
	}
	return nil
}

func externalPeersForEdge(cfg *TopologyConfig, t *Topology, edgeName string) []ExternalPeer {
	var result []ExternalPeer
	for _, ep := range cfg.ExternalPeers {
		for _, link := range t.linksForNode(edgeName) {
			other := link.ANode
			if link.ANode == edgeName {
				other = link.BNode
			}
			if other == ep.Name {
				result = append(result, ep)
			}
		}
	}
	return result
}

func sanitizeName(name string) string {
	result := make([]byte, len(name))
	for i, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			result[i] = byte(c)
		} else {
			result[i] = '_'
		}
	}
	return string(result)
}

func interfaceName(t *Topology, link Link, nodeName string) string {
	// build a per-node interface index based on link order
	idx := 0
	for _, l := range t.Links {
		if l.ANode == nodeName || l.BNode == nodeName {
			idx++
			if l.ANode == link.ANode && l.BNode == link.BNode {
				return fmt.Sprintf("eth%d", idx)
			}
		}
	}
	return "eth0"
}
