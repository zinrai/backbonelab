package main

import (
	"net"
	"testing"
)

// ---------- ipAllocator (最高優先度) ----------

func TestIPAllocator_NextSequential(t *testing.T) {
	a := newIPAllocator("10.255.0.0/16", 32)

	want := []string{
		"10.255.0.0/32",
		"10.255.0.1/32",
		"10.255.0.2/32",
		"10.255.0.3/32",
	}
	for i, w := range want {
		got, err := a.next()
		if err != nil {
			t.Fatalf("next()[%d] error: %v", i, err)
		}
		if got != w {
			t.Errorf("next()[%d] = %q, want %q", i, got, w)
		}
	}
}

func TestIPAllocator_NextOctetCarry(t *testing.T) {
	// Burn through 10.255.0.0 .. 10.255.0.255, then verify carry into 10.255.1.0
	a := newIPAllocator("10.255.0.0/16", 32)
	for i := 0; i < 256; i++ {
		if _, err := a.next(); err != nil {
			t.Fatalf("burn-in iteration %d: %v", i, err)
		}
	}
	got, err := a.next()
	if err != nil {
		t.Fatalf("next() after octet carry: %v", err)
	}
	if got != "10.255.1.0/32" {
		t.Errorf("after burning /24, next() = %q, want %q", got, "10.255.1.0/32")
	}
}

func TestIPAllocator_NextExhaustion(t *testing.T) {
	// /30 holds 4 addresses
	a := newIPAllocator("10.0.0.0/30", 32)
	for i := 0; i < 3; i++ {
		if _, err := a.next(); err != nil {
			t.Fatalf("next()[%d]: %v", i, err)
		}
	}
	// 4th increment moves current to 10.0.0.4, which is OUTSIDE the /30.
	if _, err := a.next(); err == nil {
		t.Errorf("expected exhaustion error after consuming /30, got nil")
	}
}

func TestIPAllocator_NextPairSequential(t *testing.T) {
	a := newIPAllocator("10.0.0.0/8", 30)
	want := []struct{ aIP, bIP string }{
		{"10.0.0.1/30", "10.0.0.2/30"},
		{"10.0.0.5/30", "10.0.0.6/30"},
		{"10.0.0.9/30", "10.0.0.10/30"},
		{"10.0.0.13/30", "10.0.0.14/30"},
	}
	for i, w := range want {
		ga, gb, err := a.nextPair()
		if err != nil {
			t.Fatalf("nextPair()[%d] error: %v", i, err)
		}
		if ga != w.aIP || gb != w.bIP {
			t.Errorf("nextPair()[%d] = (%q, %q), want (%q, %q)", i, ga, gb, w.aIP, w.bIP)
		}
	}
}

func TestIPAllocator_NextPairAlignsToBoundary(t *testing.T) {
	// Force the allocator to land on a non-/30-aligned address, then verify
	// nextPair() skips forward to the next /30 boundary.
	a := newIPAllocator("10.0.0.0/8", 30)
	// Advance current by 3 via next() (each next() bumps by 1).
	for i := 0; i < 3; i++ {
		if _, err := a.next(); err != nil {
			t.Fatalf("warmup next()[%d]: %v", i, err)
		}
	}
	// current is now 10.0.0.3 (not aligned to /30).
	// nextPair() should align to 10.0.0.4 and return .5/.6.
	ga, gb, err := a.nextPair()
	if err != nil {
		t.Fatalf("nextPair() after misaligned current: %v", err)
	}
	if ga != "10.0.0.5/30" || gb != "10.0.0.6/30" {
		t.Errorf("misaligned nextPair() = (%q, %q), want (10.0.0.5/30, 10.0.0.6/30)", ga, gb)
	}
}

func TestIPAllocator_NextPairExhaustion(t *testing.T) {
	// /29 (8 addresses) admits exactly one valid /30 pair under this
	// allocator: after the first pair, current advances to base+4 which is
	// still inside /29, so the bounds check passes. A second call advances
	// current to base+8, which falls outside /29 and triggers exhaustion.
	a := newIPAllocator("10.0.0.0/29", 30)
	if _, _, err := a.nextPair(); err != nil {
		t.Fatalf("first nextPair() on /29: %v", err)
	}
	if _, _, err := a.nextPair(); err == nil {
		t.Errorf("expected exhaustion after exhausting /29 pair capacity, got nil")
	}
}

func TestIncrementIP(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"10.0.0.0", 1, "10.0.0.1"},
		{"10.0.0.255", 1, "10.0.1.0"},     // octet carry
		{"10.0.255.255", 1, "10.1.0.0"},   // multi-octet carry
		{"10.255.255.255", 1, "11.0.0.0"}, // top-octet carry
		{"10.0.0.0", 256, "10.0.1.0"},     // skip by 256
		{"10.0.0.0", 4, "10.0.0.4"},       // typical /30 step
	}
	for _, c := range cases {
		ip := net.ParseIP(c.in).To4()
		incrementIP(ip, c.n)
		if got := ip.String(); got != c.want {
			t.Errorf("incrementIP(%s, %d) = %s, want %s", c.in, c.n, got, c.want)
		}
	}
}

func TestAlignTo30(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"10.0.0.0", "10.0.0.0"}, // already aligned
		{"10.0.0.1", "10.0.0.4"}, // +3
		{"10.0.0.2", "10.0.0.4"}, // +2
		{"10.0.0.3", "10.0.0.4"}, // +1
		{"10.0.0.4", "10.0.0.4"}, // already aligned
		{"10.0.0.252", "10.0.0.252"},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.in).To4()
		alignTo30(ip)
		if got := ip.String(); got != c.want {
			t.Errorf("alignTo30(%s) = %s, want %s", c.in, got, c.want)
		}
	}
}

// ---------- planEdges (高優先度) ----------

func TestPlanEdges(t *testing.T) {
	cfg := &TopologyConfig{
		Sites: []Site{
			{Name: "hkd"}, // 1 customer
			{Name: "tyo"}, // transit + peer + customer = 3 peers
			{Name: "kyu"}, // no external peers
		},
		ExternalPeers: []ExternalPeer{
			{Name: "transit-a", Type: TypeTransit, ConnectTo: "tyo"},
			{Name: "peer-a", Type: TypePeer, ConnectTo: "tyo"},
			{Name: "customer-a", Type: TypeCustomer, ConnectTo: "hkd"},
			{Name: "customer-b", Type: TypeCustomer, ConnectTo: "tyo"},
		},
	}
	plans := planEdges(cfg)

	cases := []struct {
		site      string
		wantCount int
	}{
		{"hkd", 1}, // 1 customer
		{"tyo", 3}, // transit + peer + customer
		{"kyu", 1}, // none → still 1
	}
	for _, c := range cases {
		p, ok := plans[c.site]
		if !ok {
			t.Errorf("planEdges missing site %s", c.site)
			continue
		}
		if p.count != c.wantCount {
			t.Errorf("planEdges[%s].count = %d, want %d", c.site, p.count, c.wantCount)
		}
	}

	// Each peer should map to a unique edge index within its site, 1..count.
	for _, ep := range cfg.ExternalPeers {
		idx, ok := plans[ep.ConnectTo].peerToEdge[ep.Name]
		if !ok {
			t.Errorf("peer %s not mapped to an edge", ep.Name)
			continue
		}
		if idx < 1 || idx > plans[ep.ConnectTo].count {
			t.Errorf("peer %s maps to edge index %d, out of range [1, %d]", ep.Name, idx, plans[ep.ConnectTo].count)
		}
	}
}

// ---------- deriveIBGPNeighbors (高優先度) ----------

func TestDeriveIBGPNeighbors(t *testing.T) {
	// Build a minimal 2-site topology by hand to exercise the RR mesh logic
	// without going through buildTopology.
	t.Helper()
	topo := &Topology{
		Nodes: map[string]*Node{
			"hkd-core1": {Name: "hkd-core1", Site: "hkd", Role: RoleCore, Loopback: "10.255.0.0/32"},
			"hkd-core2": {Name: "hkd-core2", Site: "hkd", Role: RoleCore, Loopback: "10.255.0.1/32"},
			"hkd-rr1":   {Name: "hkd-rr1", Site: "hkd", Role: RoleRR, Loopback: "10.255.0.2/32"},
			"hkd-rr2":   {Name: "hkd-rr2", Site: "hkd", Role: RoleRR, Loopback: "10.255.0.3/32"},
			"hkd-edge1": {Name: "hkd-edge1", Site: "hkd", Role: RoleEdge, Loopback: "10.255.0.4/32"},
			"tyo-core1": {Name: "tyo-core1", Site: "tyo", Role: RoleCore, Loopback: "10.255.0.5/32"},
			"tyo-core2": {Name: "tyo-core2", Site: "tyo", Role: RoleCore, Loopback: "10.255.0.6/32"},
			"tyo-rr1":   {Name: "tyo-rr1", Site: "tyo", Role: RoleRR, Loopback: "10.255.0.7/32"},
			"tyo-rr2":   {Name: "tyo-rr2", Site: "tyo", Role: RoleRR, Loopback: "10.255.0.8/32"},
			"tyo-edge1": {Name: "tyo-edge1", Site: "tyo", Role: RoleEdge, Loopback: "10.255.0.9/32"},
		},
		RRClusters: []RRCluster{
			{
				ClusterID: "10.255.0.2",
				Members:   []string{"hkd-rr1", "hkd-rr2"},
				Clients:   []string{"hkd-core1", "hkd-core2", "hkd-edge1"},
			},
			{
				ClusterID: "10.255.0.7",
				Members:   []string{"tyo-rr1", "tyo-rr2"},
				Clients:   []string{"tyo-core1", "tyo-core2", "tyo-edge1"},
			},
		},
		IBGPNeighbors: make(map[string][]IBGPNeighbor),
	}

	if err := topo.deriveIBGPNeighbors(); err != nil {
		t.Fatalf("deriveIBGPNeighbors: %v", err)
	}

	// Property 1: each RR has every OTHER RR (different cluster) + its own clients.
	// hkd-rr1 expected neighbors: tyo-rr1, tyo-rr2 (cross-cluster), hkd-core1, hkd-core2, hkd-edge1 (clients)
	wantNeighbors := map[string][]string{
		"hkd-rr1":   {"tyo-rr1", "tyo-rr2", "hkd-core1", "hkd-core2", "hkd-edge1"},
		"hkd-rr2":   {"tyo-rr1", "tyo-rr2", "hkd-core1", "hkd-core2", "hkd-edge1"},
		"tyo-rr1":   {"hkd-rr1", "hkd-rr2", "tyo-core1", "tyo-core2", "tyo-edge1"},
		"tyo-rr2":   {"hkd-rr1", "hkd-rr2", "tyo-core1", "tyo-core2", "tyo-edge1"},
		"hkd-core1": {"hkd-rr1", "hkd-rr2"},
		"hkd-core2": {"hkd-rr1", "hkd-rr2"},
		"hkd-edge1": {"hkd-rr1", "hkd-rr2"},
		"tyo-core1": {"tyo-rr1", "tyo-rr2"},
		"tyo-core2": {"tyo-rr1", "tyo-rr2"},
		"tyo-edge1": {"tyo-rr1", "tyo-rr2"},
	}

	for node, want := range wantNeighbors {
		gotSet := make(map[string]bool)
		for _, nb := range topo.IBGPNeighbors[node] {
			gotSet[nb.Name] = true
		}
		if len(gotSet) != len(want) {
			t.Errorf("%s has %d neighbors, want %d (got %v)", node, len(gotSet), len(want), gotSet)
		}
		for _, w := range want {
			if !gotSet[w] {
				t.Errorf("%s missing expected neighbor %s", node, w)
			}
		}
	}

	// Property 2: same-cluster RRs do NOT have each other as neighbors.
	for _, pair := range [][2]string{{"hkd-rr1", "hkd-rr2"}, {"tyo-rr1", "tyo-rr2"}} {
		for _, nb := range topo.IBGPNeighbors[pair[0]] {
			if nb.Name == pair[1] {
				t.Errorf("intra-cluster RR pair (%s, %s) should not be iBGP neighbors", pair[0], pair[1])
			}
		}
	}

	// Property 3: each iBGP neighbor entry carries the peer loopback address (no /32 suffix).
	for node, nbs := range topo.IBGPNeighbors {
		for _, nb := range nbs {
			peerNode, ok := topo.Nodes[nb.Name]
			if !ok {
				t.Errorf("%s lists unknown neighbor %s", node, nb.Name)
				continue
			}
			wantAddr := stripPrefix(peerNode.Loopback)
			if nb.Address != wantAddr {
				t.Errorf("%s neighbor %s address = %s, want %s", node, nb.Name, nb.Address, wantAddr)
			}
		}
	}
}

// ---------- buildTopology smoke test (高優先度) ----------

func TestBuildTopology_Smoke(t *testing.T) {
	cfg := &TopologyConfig{
		ASN: 65000,
		ExternalASN: ExternalASN{
			TransitStart:  65001,
			PeerStart:     65010,
			CustomerStart: 65100,
		},
		Loopback: "10.255.0.0/16",
		Backbone: "10.0.0.0/8",
		Sites: []Site{
			{Name: "hkd"},
			{Name: "tyo"},
		},
		BackboneLinks: [][]string{{"hkd", "tyo"}},
		ExternalPeers: []ExternalPeer{
			{Name: "transit-a", Type: TypeTransit, ConnectTo: "tyo"},
			{Name: "customer-a", Type: TypeCustomer, ConnectTo: "hkd"},
		},
	}

	topo, err := buildTopology(cfg)
	if err != nil {
		t.Fatalf("buildTopology: %v", err)
	}

	// 2 sites × (2 core + 2 rr) = 8 internal nodes, fixed.
	// edges: hkd has 1 customer → 1 edge; tyo has 1 transit → 1 edge. Total 2.
	// external: 2 (transit-a, customer-a).
	wantTotal := 8 + 2 + 2
	if got := len(topo.Nodes); got != wantTotal {
		t.Errorf("total nodes = %d, want %d", got, wantTotal)
	}

	// Count by role.
	roleCount := map[string]int{}
	for _, n := range topo.Nodes {
		roleCount[n.Role]++
	}
	wantRole := map[string]int{
		RoleCore:     4, // 2 sites × 2
		RoleRR:       4,
		RoleEdge:     2, // hkd + tyo
		RoleExternal: 2,
	}
	for role, want := range wantRole {
		if got := roleCount[role]; got != want {
			t.Errorf("role %s count = %d, want %d", role, got, want)
		}
	}

	// ASN assignment for external peers.
	if topo.Nodes["transit-a"].ASN != 65001 {
		t.Errorf("transit-a ASN = %d, want 65001", topo.Nodes["transit-a"].ASN)
	}
	if topo.Nodes["customer-a"].ASN != 65100 {
		t.Errorf("customer-a ASN = %d, want 65100", topo.Nodes["customer-a"].ASN)
	}

	// Loopback uniqueness across all nodes.
	seen := map[string]string{}
	for _, n := range topo.Nodes {
		if prev, ok := seen[n.Loopback]; ok {
			t.Errorf("duplicate loopback %s on nodes %s and %s", n.Loopback, prev, n.Name)
		}
		seen[n.Loopback] = n.Name
	}

	// RR cluster: one per site.
	if got := len(topo.RRClusters); got != 2 {
		t.Errorf("RRClusters = %d, want 2", got)
	}

	// IBGPNeighbors populated for every internal node + RR.
	for _, n := range topo.Nodes {
		if n.Role == RoleExternal {
			continue
		}
		if len(topo.IBGPNeighbors[n.Name]) == 0 {
			t.Errorf("%s (role %s) has no iBGP neighbors", n.Name, n.Role)
		}
	}
}
