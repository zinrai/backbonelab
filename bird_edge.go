package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
)

//go:embed templates/bird_edge.tmpl
var birdEdgeTmpl string

type ebgpSession struct {
	SanitizedName string
	PeerAddr      string
	PeerASN       int
	ImportFilter  string
	ExportFilter  string
}

type edgeData struct {
	RouterID       string
	Loopback       string
	ASN            int
	Filters        string
	OSPFInterfaces []ospfInterface
	IBGPNeighbors  []ibgpNeighborEntry
	EBGPSessions   []ebgpSession
}

func generateBirdEdge(node *Node, t *Topology, externalPeers []ExternalPeer) string {
	loopbackIP := stripPrefix(node.Loopback)

	var ospfIfaces []ospfInterface
	for _, link := range t.ospfLinksForNode(node.Name) {
		iface := interfaceName(t, link, node.Name)
		addr := t.addrForNode(link, node.Name)
		ospfIfaces = append(ospfIfaces, ospfInterface{Name: iface, Address: addr})
	}

	var ibgpNeighbors []ibgpNeighborEntry
	for _, nb := range t.IBGPNeighbors[node.Name] {
		ibgpNeighbors = append(ibgpNeighbors, ibgpNeighborEntry{
			SanitizedName: sanitizeName(nb.Name),
			Address:       nb.Address,
			IsRRClient:    false,
		})
	}

	var ebgpSessions []ebgpSession
	for _, ep := range externalPeers {
		extNode := t.Nodes[ep.Name]
		var peerAddr string
		for _, link := range t.linksForNode(node.Name) {
			other := link.ANode
			if link.ANode == node.Name {
				other = link.BNode
			}
			if other == ep.Name {
				peerAddr = t.peerAddrForNode(link, node.Name)
				break
			}
		}
		if peerAddr == "" {
			continue
		}
		var importFilter, exportFilter string
		switch ep.Type {
		case TypeTransit:
			importFilter = "bgp_import_transit"
			exportFilter = "bgp_export_to_transit"
		case TypePeer:
			importFilter = "bgp_import_peer"
			exportFilter = "bgp_export_to_peer"
		case TypeCustomer:
			importFilter = "bgp_import_customer"
			exportFilter = "bgp_export_to_customer"
		}
		ebgpSessions = append(ebgpSessions, ebgpSession{
			SanitizedName: sanitizeName(ep.Name),
			PeerAddr:      peerAddr,
			PeerASN:       extNode.ASN,
			ImportFilter:  importFilter,
			ExportFilter:  exportFilter,
		})
	}

	data := edgeData{
		RouterID:       loopbackIP,
		Loopback:       node.Loopback,
		ASN:            t.Cfg.ASN,
		Filters:        generateBirdFilters(t),
		OSPFInterfaces: ospfIfaces,
		IBGPNeighbors:  ibgpNeighbors,
		EBGPSessions:   ebgpSessions,
	}

	tmpl := template.Must(template.New("edge").Parse(birdEdgeTmpl))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("failed to render bird_edge.tmpl: %v", err))
	}
	return buf.String()
}
