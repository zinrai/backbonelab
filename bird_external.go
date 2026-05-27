package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
)

//go:embed templates/bird_external_transit.tmpl
var birdExternalTransitTmpl string

//go:embed templates/bird_external_peer.tmpl
var birdExternalPeerTmpl string

//go:embed templates/bird_external_customer.tmpl
var birdExternalCustomerTmpl string

type externalData struct {
	RouterID     string
	Loopback     string
	ASN          int
	EBGPSessions []ebgpSession
}

func generateBirdExternal(node *Node, t *Topology) string {
	loopbackIP := stripPrefix(node.Loopback)

	var ebgpSessions []ebgpSession
	for _, link := range t.linksForNode(node.Name) {
		other := link.ANode
		if link.ANode == node.Name {
			other = link.BNode
		}
		otherNode := t.Nodes[other]
		if otherNode.Role != RoleEdge {
			continue
		}
		peerAddr := t.peerAddrForNode(link, node.Name)
		ebgpSessions = append(ebgpSessions, ebgpSession{
			SanitizedName: sanitizeName(other),
			PeerAddr:      peerAddr,
			PeerASN:       t.Cfg.ASN,
		})
	}

	data := externalData{
		RouterID:     loopbackIP,
		Loopback:     node.Loopback,
		ASN:          node.ASN,
		EBGPSessions: ebgpSessions,
	}

	var tmplStr string
	switch node.PeerType {
	case TypeTransit:
		tmplStr = birdExternalTransitTmpl
	case TypePeer:
		tmplStr = birdExternalPeerTmpl
	case TypeCustomer:
		tmplStr = birdExternalCustomerTmpl
	default:
		panic(fmt.Sprintf("unknown peer type for external node %s: %s", node.Name, node.PeerType))
	}

	tmpl := template.Must(template.New("external").Parse(tmplStr))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("failed to render bird_external tmpl for %s: %v", node.Name, err))
	}
	return buf.String()
}
