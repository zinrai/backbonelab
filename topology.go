package main

import (
	"fmt"
	"net"
)

const (
	RoleCore     = "core"
	RoleRR       = "rr"
	RoleEdge     = "edge"
	RoleExternal = "external"
)

const (
	TypeTransit  = "transit"
	TypePeer     = "peer"
	TypeCustomer = "customer"
)

const (
	CommunityTransit  = 100
	CommunityPeer     = 200
	CommunityCustomer = 300
	CommunityOwn      = 900

	LocalPrefTransit = 100
	LocalPrefPeer    = 200
)

type Node struct {
	Name     string
	Site     string
	Role     string
	Loopback string // e.g. 10.255.0.1/32
	ASN      int
	PeerType string // for external nodes: transit, peer, customer
}

type Link struct {
	ANode string
	BNode string
	AAddr string // e.g. 10.0.0.1/30
	BAddr string // e.g. 10.0.0.2/30
	MPLS  bool
}

type RRCluster struct {
	ClusterID string
	Members   []string // rr node names
	Clients   []string // core + edge node names in the same site
}

type IBGPNeighbor struct {
	Name    string
	Address string // loopback address without prefix length
}

type Topology struct {
	Nodes         map[string]*Node
	Links         []Link
	RRClusters    []RRCluster
	IBGPNeighbors map[string][]IBGPNeighbor // node name -> list of iBGP neighbors
	Cfg           *TopologyConfig
}

// edgePlan holds the edge assignment plan for a site.
// Each external peer gets its own dedicated edge node (1 peer = 1 edge).
// This applies equally to transit, peer, and customer types, reflecting
// real ISP practice where each upstream/peer/customer connection has its
// own edge router. A site with no external peers still gets one edge node.
type edgePlan struct {
	count      int            // total edge nodes
	peerToEdge map[string]int // external peer name -> edge index
}

func planEdges(cfg *TopologyConfig) map[string]*edgePlan {
	plans := make(map[string]*edgePlan)

	for _, ep := range cfg.ExternalPeers {
		if plans[ep.ConnectTo] == nil {
			plans[ep.ConnectTo] = &edgePlan{
				count:      0,
				peerToEdge: make(map[string]int),
			}
		}
		plan := plans[ep.ConnectTo]
		plan.count++
		plan.peerToEdge[ep.Name] = plan.count
	}

	// ensure every site has at least one edge
	for _, site := range cfg.Sites {
		if plans[site.Name] == nil {
			plans[site.Name] = &edgePlan{
				count:      1,
				peerToEdge: make(map[string]int),
			}
		}
	}

	return plans
}

func buildTopology(cfg *TopologyConfig) (*Topology, error) {
	t := &Topology{
		Nodes:         make(map[string]*Node),
		IBGPNeighbors: make(map[string][]IBGPNeighbor),
		Cfg:           cfg,
	}

	loopbackAlloc := newIPAllocator(cfg.Loopback, 32)
	linkAlloc := newIPAllocator(cfg.Backbone, 30)

	edgePlans := planEdges(cfg)

	edgeCountForSite := func(siteName string) int {
		if p, ok := edgePlans[siteName]; ok {
			return p.count
		}
		return 1
	}

	// build internal nodes
	for _, site := range cfg.Sites {
		// core nodes
		for j := 1; j <= 2; j++ {
			lo, err := loopbackAlloc.next()
			if err != nil {
				return nil, err
			}
			name := fmt.Sprintf("%s-core%d", site.Name, j)
			t.Nodes[name] = &Node{
				Name:     name,
				Site:     site.Name,
				Role:     RoleCore,
				Loopback: lo,
				ASN:      cfg.ASN,
			}
		}

		// rr nodes
		for j := 1; j <= 2; j++ {
			lo, err := loopbackAlloc.next()
			if err != nil {
				return nil, err
			}
			name := fmt.Sprintf("%s-rr%d", site.Name, j)
			t.Nodes[name] = &Node{
				Name:     name,
				Site:     site.Name,
				Role:     RoleRR,
				Loopback: lo,
				ASN:      cfg.ASN,
			}
		}

		// edge nodes
		for j := 1; j <= edgeCountForSite(site.Name); j++ {
			lo, err := loopbackAlloc.next()
			if err != nil {
				return nil, err
			}
			name := fmt.Sprintf("%s-edge%d", site.Name, j)
			t.Nodes[name] = &Node{
				Name:     name,
				Site:     site.Name,
				Role:     RoleEdge,
				Loopback: lo,
				ASN:      cfg.ASN,
			}
		}
	}

	// build external nodes
	transitASN := cfg.ExternalASN.TransitStart
	peerASN := cfg.ExternalASN.PeerStart
	customerASN := cfg.ExternalASN.CustomerStart
	for _, ep := range cfg.ExternalPeers {
		var asn int
		switch ep.Type {
		case TypeTransit:
			asn = transitASN
			transitASN++
		case TypePeer:
			asn = peerASN
			peerASN++
		case TypeCustomer:
			asn = customerASN
			customerASN++
		}
		lo, err := loopbackAlloc.next()
		if err != nil {
			return nil, err
		}
		t.Nodes[ep.Name] = &Node{
			Name:     ep.Name,
			Site:     ep.ConnectTo,
			Role:     RoleExternal,
			Loopback: lo,
			ASN:      asn,
			PeerType: ep.Type,
		}
	}

	// build backbone links (inter-site: core1<->core1, core2<->core2)
	for _, bl := range cfg.BackboneLinks {
		siteA, siteB := bl[0], bl[1]
		for j := 1; j <= 2; j++ {
			aNode := fmt.Sprintf("%s-core%d", siteA, j)
			bNode := fmt.Sprintf("%s-core%d", siteB, j)
			aAddr, bAddr, err := linkAlloc.nextPair()
			if err != nil {
				return nil, err
			}
			t.Links = append(t.Links, Link{
				ANode: aNode,
				BNode: bNode,
				AAddr: aAddr,
				BAddr: bAddr,
				MPLS:  true,
			})
		}
	}

	// build intra-site links
	for _, site := range cfg.Sites {
		// core1 <-> core2
		{
			aAddr, bAddr, err := linkAlloc.nextPair()
			if err != nil {
				return nil, err
			}
			t.Links = append(t.Links, Link{
				ANode: fmt.Sprintf("%s-core1", site.Name),
				BNode: fmt.Sprintf("%s-core2", site.Name),
				AAddr: aAddr,
				BAddr: bAddr,
				MPLS:  true,
			})
		}

		// core1,core2 <-> rr1,rr2
		for _, coreIdx := range []int{1, 2} {
			for _, rrIdx := range []int{1, 2} {
				aAddr, bAddr, err := linkAlloc.nextPair()
				if err != nil {
					return nil, err
				}
				t.Links = append(t.Links, Link{
					ANode: fmt.Sprintf("%s-core%d", site.Name, coreIdx),
					BNode: fmt.Sprintf("%s-rr%d", site.Name, rrIdx),
					AAddr: aAddr,
					BAddr: bAddr,
					MPLS:  false,
				})
			}
		}

		// core1,core2 <-> edge nodes
		for _, coreIdx := range []int{1, 2} {
			for edgeIdx := 1; edgeIdx <= edgeCountForSite(site.Name); edgeIdx++ {
				aAddr, bAddr, err := linkAlloc.nextPair()
				if err != nil {
					return nil, err
				}
				t.Links = append(t.Links, Link{
					ANode: fmt.Sprintf("%s-core%d", site.Name, coreIdx),
					BNode: fmt.Sprintf("%s-edge%d", site.Name, edgeIdx),
					AAddr: aAddr,
					BAddr: bAddr,
					MPLS:  true,
				})
			}
		}
	}

	// build external peer links using edgePlan
	for _, ep := range cfg.ExternalPeers {
		plan := edgePlans[ep.ConnectTo]
		edgeIdx := plan.peerToEdge[ep.Name]
		edgeNode := fmt.Sprintf("%s-edge%d", ep.ConnectTo, edgeIdx)
		aAddr, bAddr, err := linkAlloc.nextPair()
		if err != nil {
			return nil, err
		}
		t.Links = append(t.Links, Link{
			ANode: edgeNode,
			BNode: ep.Name,
			AAddr: aAddr,
			BAddr: bAddr,
			MPLS:  false,
		})
	}

	// build RR clusters
	for _, site := range cfg.Sites {
		cluster := RRCluster{
			ClusterID: stripPrefix(t.Nodes[fmt.Sprintf("%s-rr1", site.Name)].Loopback),
		}
		for j := 1; j <= 2; j++ {
			cluster.Members = append(cluster.Members, fmt.Sprintf("%s-rr%d", site.Name, j))
		}
		for j := 1; j <= 2; j++ {
			cluster.Clients = append(cluster.Clients, fmt.Sprintf("%s-core%d", site.Name, j))
		}
		for j := 1; j <= edgeCountForSite(site.Name); j++ {
			cluster.Clients = append(cluster.Clients, fmt.Sprintf("%s-edge%d", site.Name, j))
		}
		t.RRClusters = append(t.RRClusters, cluster)
	}

	// derive iBGP neighbors
	if err := t.deriveIBGPNeighbors(); err != nil {
		return nil, err
	}

	return t, nil
}

func (t *Topology) deriveIBGPNeighbors() error {
	// for each RR cluster member: neighbors are all other RR members (inter-cluster full mesh) + clients
	// for each client: neighbors are the RR members of its cluster

	// build cluster membership lookup
	nodeToCluster := make(map[string]*RRCluster)
	for i := range t.RRClusters {
		cl := &t.RRClusters[i]
		for _, m := range cl.Members {
			nodeToCluster[m] = cl
		}
		for _, c := range cl.Clients {
			nodeToCluster[c] = cl
		}
	}

	// RR inter-cluster full mesh
	rrNodes := []string{}
	for _, cl := range t.RRClusters {
		rrNodes = append(rrNodes, cl.Members...)
	}
	for _, a := range rrNodes {
		for _, b := range rrNodes {
			if a == b {
				continue
			}
			// skip if same cluster
			if nodeToCluster[a] == nodeToCluster[b] {
				continue
			}
			t.IBGPNeighbors[a] = append(t.IBGPNeighbors[a], IBGPNeighbor{
				Name:    b,
				Address: stripPrefix(t.Nodes[b].Loopback),
			})
		}
	}

	// RR <-> clients within same cluster
	for _, cl := range t.RRClusters {
		for _, member := range cl.Members {
			for _, client := range cl.Clients {
				t.IBGPNeighbors[member] = append(t.IBGPNeighbors[member], IBGPNeighbor{
					Name:    client,
					Address: stripPrefix(t.Nodes[client].Loopback),
				})
				t.IBGPNeighbors[client] = append(t.IBGPNeighbors[client], IBGPNeighbor{
					Name:    member,
					Address: stripPrefix(t.Nodes[member].Loopback),
				})
			}
		}
	}

	return nil
}

func (t *Topology) linksForNode(nodeName string) []Link {
	var result []Link
	for _, l := range t.Links {
		if l.ANode == nodeName || l.BNode == nodeName {
			result = append(result, l)
		}
	}
	return result
}

func (t *Topology) addrForNode(link Link, nodeName string) string {
	if link.ANode == nodeName {
		return link.AAddr
	}
	return link.BAddr
}

func (t *Topology) peerAddrForNode(link Link, nodeName string) string {
	if link.ANode == nodeName {
		return stripPrefix(link.BAddr)
	}
	return stripPrefix(link.AAddr)
}

// ipAllocator allocates IP addresses sequentially from a CIDR
type ipAllocator struct {
	network *net.IPNet
	current net.IP
	prefix  int
}

func newIPAllocator(cidr string, prefix int) *ipAllocator {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(fmt.Sprintf("invalid CIDR: %s", cidr))
	}
	return &ipAllocator{
		network: network,
		current: cloneIP(network.IP),
		prefix:  prefix,
	}
}

func (a *ipAllocator) next() (string, error) {
	// for /32 loopback: increment by 1
	ip := cloneIP(a.current)
	incrementIP(a.current, 1)
	if !a.network.Contains(a.current) {
		return "", fmt.Errorf("IP address pool exhausted in %s", a.network)
	}
	return fmt.Sprintf("%s/%d", ip.String(), a.prefix), nil
}

func (a *ipAllocator) nextPair() (string, string, error) {
	// for /30 links: .1 and .2 of each /30 block
	// align to /30 boundary first
	alignTo30(a.current)
	base := cloneIP(a.current)

	aIP := cloneIP(base)
	incrementIP(aIP, 1)
	bIP := cloneIP(base)
	incrementIP(bIP, 2)
	incrementIP(a.current, 4)

	if !a.network.Contains(a.current) {
		return "", "", fmt.Errorf("IP address pool exhausted in %s", a.network)
	}

	return fmt.Sprintf("%s/30", aIP.String()), fmt.Sprintf("%s/30", bIP.String()), nil
}

func alignTo30(ip net.IP) {
	ip4 := ip.To4()
	if ip4 == nil {
		return
	}
	last := ip4[3]
	remainder := last % 4
	if remainder != 0 {
		ip4[3] = last + (4 - remainder)
	}
}

func cloneIP(ip net.IP) net.IP {
	clone := make(net.IP, len(ip))
	copy(clone, ip)
	return clone
}

func incrementIP(ip net.IP, n int) {
	ip4 := ip.To4()
	if ip4 == nil {
		return
	}
	val := int(ip4[0])<<24 | int(ip4[1])<<16 | int(ip4[2])<<8 | int(ip4[3])
	val += n
	ip4[0] = byte(val >> 24)
	ip4[1] = byte(val >> 16)
	ip4[2] = byte(val >> 8)
	ip4[3] = byte(val)
}

func stripPrefix(cidr string) string {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return cidr
	}
	return ip.String()
}

// ospfLinksForNode returns the links a node should include in OSPF: every link
// except those toward external-AS nodes (which are eBGP attachments).
// core, rr, and edge nodes all participate in OSPF.
func (t *Topology) ospfLinksForNode(nodeName string) []Link {
	var result []Link
	for _, link := range t.linksForNode(nodeName) {
		other := link.ANode
		if link.ANode == nodeName {
			other = link.BNode
		}
		if t.Nodes[other].Role == RoleExternal {
			continue
		}
		result = append(result, link)
	}
	return result
}
