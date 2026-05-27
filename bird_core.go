package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
)

//go:embed templates/bird_core.tmpl
var birdCoreTmpl string

type ospfInterface struct {
	Name    string
	Address string
}

type ibgpNeighborEntry struct {
	SanitizedName string
	Address       string
	IsRRClient    bool
}

type coreData struct {
	RouterID       string
	Loopback       string
	ASN            int
	Filters        string
	OSPFInterfaces []ospfInterface
	IBGPNeighbors  []ibgpNeighborEntry
}

func generateBirdCore(node *Node, t *Topology) string {
	loopbackIP := stripPrefix(node.Loopback)

	var ospfIfaces []ospfInterface
	for _, link := range t.ospfLinksForNode(node.Name) {
		iface := interfaceName(t, link, node.Name)
		addr := t.addrForNode(link, node.Name)
		ospfIfaces = append(ospfIfaces, ospfInterface{Name: iface, Address: addr})
	}

	var ibgpNeighbors []ibgpNeighborEntry
	for _, nb := range t.IBGPNeighbors[node.Name] {
		nbNode := t.Nodes[nb.Name]
		ibgpNeighbors = append(ibgpNeighbors, ibgpNeighborEntry{
			SanitizedName: sanitizeName(nb.Name),
			Address:       nb.Address,
			IsRRClient:    nbNode.Role == RoleRR,
		})
	}

	data := coreData{
		RouterID:       loopbackIP,
		Loopback:       node.Loopback,
		ASN:            t.Cfg.ASN,
		Filters:        generateBirdFilters(t),
		OSPFInterfaces: ospfIfaces,
		IBGPNeighbors:  ibgpNeighbors,
	}

	tmpl := template.Must(template.New("core").Parse(birdCoreTmpl))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("failed to render bird_core.tmpl: %v", err))
	}
	return buf.String()
}
