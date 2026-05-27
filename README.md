# backbonelab

A config generator for building a backbone-scale routing lab on top of [containerlab](https://containerlab.dev/) + [BIRD](https://bird.network.cz/).

From a single topology YAML, backbonelab produces:

- a containerlab `topology.yaml` (nodes, links, interface bring-up)
- a per-node `bird.conf` (OSPF, iBGP with Route Reflectors, eBGP with import/export policy)

The focus is on configurations where the routing protocols (OSPF / iBGP / eBGP) are the subject of study. Hosts and servers are intentionally absent.

See [DESIGN.md](./DESIGN.md) for the design rationale.

## Requirements

- [containerlab](https://containerlab.dev/) (for running the generated lab)
- Docker (containerlab's runtime)
- Docker image: `ghcr.io/zinrai/docker-ubuntu-bird3:ubuntu-resolute` (BIRD 3.2.0 on Ubuntu; pulled automatically by containerlab)

## Usage

```sh
backbonelab [--output DIR] <topology.yaml>
```

Example:

```sh
./backbonelab --output ./output topology.yaml
```

This writes:

```
output/
  .clab.yaml            # containerlab topology
  hkd-core1/bird.conf
  hkd-core2/bird.conf
  tyo-core1/bird.conf
  ...
```

Deploy the generated lab with containerlab:

```sh
cd output
sudo containerlab deploy -t .clab.yaml
```

### CLI Options

| Option | Default | Description |
|--------|---------|-------------|
| `--output` | `./output` | Output directory |

The only CLI option is `--output`. Everything else that influences the lab (own ASN, external ASN starting values, address ranges) lives in the topology YAML, so a single YAML file fully reproduces the lab.

## Topology YAML

A minimal example is bundled as [topology.yaml](./topology.yaml).

```yaml
asn: 65000                  # own ASN (default: 65000)
external_asn:
  transit_start: 65001      # starting Transit ASN (default: 65001)
  peer_start: 65010         # starting Peer ASN (default: 65010)
  customer_start: 65100     # starting Customer ASN (default: 65100)
loopback: 10.255.0.0/16     # loopback address range (default: 10.255.0.0/16)
backbone: 10.0.0.0/8        # inter-site link address range (default: 10.0.0.0/8)

sites:
  - name: hkd
  - name: tyo
  - name: osa
  - name: kyu

backbone_links:
  - [hkd, tyo]
  - [tyo, osa]
  - [osa, kyu]

external_peers:
  - name: transit-a
    type: transit
    connect_to: tyo
  - name: peer-a
    type: peer
    connect_to: tyo
  - name: customer-a
    type: customer
    connect_to: hkd
```

The top-level network-design fields (`asn`, `external_asn`, `loopback`, `backbone`) are all optional and fall back to the defaults above when omitted. The structural fields (`sites`, `backbone_links`, `external_peers`) are required.

### Sites and node roles

Each site automatically gets three layers:

| Role | Count per site | Role |
|------|----------------|------|
| core | 2 | Inter-site backbone, OSPF |
| rr | 2 | iBGP Route Reflector |
| edge | one per external peer (1 if none) | External AS attachment (transit/peer/customer) |

Edge count is derived from `external_peers`: one external peer = one dedicated edge router. transit / peer / customer differ in operational importance, blast radius, and filter policy, so each gets its own edge.

### External peer types

| Type | Meaning |
|------|---------|
| `transit` | Upstream ISP (we depend on them; receive default route / full table) |
| `peer` | Lateral attachment such as at an IX (exchange own routes only) |
| `customer` | An organization buying connectivity from us (their own AS) |

## License

This project is licensed under the [MIT License](./LICENSE).
