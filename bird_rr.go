package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
)

//go:embed templates/bird_rr.tmpl
var birdRRTmpl string

type rrData struct {
	RouterID       string
	Loopback       string
	ASN            int
	ClusterID      string
	Filters        string
	OSPFInterfaces []ospfInterface
	IBGPNeighbors  []ibgpNeighborEntry
}

func generateBirdRR(node *Node, t *Topology, cluster *RRCluster) string {
	loopbackIP := stripPrefix(node.Loopback)

	var ospfIfaces []ospfInterface
	for _, link := range t.ospfLinksForNode(node.Name) {
		iface := interfaceName(t, link, node.Name)
		addr := t.addrForNode(link, node.Name)
		ospfIfaces = append(ospfIfaces, ospfInterface{Name: iface, Address: addr})
	}

	clientSet := make(map[string]bool)
	for _, c := range cluster.Clients {
		clientSet[c] = true
	}

	var ibgpNeighbors []ibgpNeighborEntry
	for _, nb := range t.IBGPNeighbors[node.Name] {
		ibgpNeighbors = append(ibgpNeighbors, ibgpNeighborEntry{
			SanitizedName: sanitizeName(nb.Name),
			Address:       nb.Address,
			IsRRClient:    clientSet[nb.Name],
		})
	}

	data := rrData{
		RouterID:       loopbackIP,
		Loopback:       node.Loopback,
		ASN:            t.Cfg.ASN,
		ClusterID:      cluster.ClusterID,
		Filters:        generateBirdFilters(t),
		OSPFInterfaces: ospfIfaces,
		IBGPNeighbors:  ibgpNeighbors,
	}

	tmpl := template.Must(template.New("rr").Parse(birdRRTmpl))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("failed to render bird_rr.tmpl: %v", err))
	}
	return buf.String()
}
