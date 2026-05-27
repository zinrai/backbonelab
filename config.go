package main

import (
	"fmt"
	"net"
	"os"

	"github.com/goccy/go-yaml"
)

type Site struct {
	Name string `yaml:"name"`
}

type ExternalPeer struct {
	Name      string `yaml:"name"`
	Type      string `yaml:"type"`       // transit, peer, customer
	ConnectTo string `yaml:"connect_to"` // site name
}

type ExternalASN struct {
	TransitStart  int `yaml:"transit_start"`
	PeerStart     int `yaml:"peer_start"`
	CustomerStart int `yaml:"customer_start"`
}

type TopologyConfig struct {
	ASN           int            `yaml:"asn"`
	ExternalASN   ExternalASN    `yaml:"external_asn"`
	Loopback      string         `yaml:"loopback"`
	Backbone      string         `yaml:"backbone"`
	Sites         []Site         `yaml:"sites"`
	BackboneLinks [][]string     `yaml:"backbone_links"`
	ExternalPeers []ExternalPeer `yaml:"external_peers"`
}

func loadConfig(path string) (*TopologyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg TopologyConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	applyDefaults(&cfg)

	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func applyDefaults(cfg *TopologyConfig) {
	if cfg.ASN == 0 {
		cfg.ASN = 65000
	}
	if cfg.ExternalASN.TransitStart == 0 {
		cfg.ExternalASN.TransitStart = 65001
	}
	if cfg.ExternalASN.PeerStart == 0 {
		cfg.ExternalASN.PeerStart = 65010
	}
	if cfg.ExternalASN.CustomerStart == 0 {
		cfg.ExternalASN.CustomerStart = 65100
	}
	if cfg.Loopback == "" {
		cfg.Loopback = "10.255.0.0/16"
	}
	if cfg.Backbone == "" {
		cfg.Backbone = "10.0.0.0/8"
	}
}

func validateConfig(cfg *TopologyConfig) error {
	if cfg.ASN <= 0 {
		return fmt.Errorf("asn must be positive")
	}
	if cfg.ExternalASN.TransitStart <= 0 {
		return fmt.Errorf("external_asn.transit_start must be positive")
	}
	if cfg.ExternalASN.PeerStart <= 0 {
		return fmt.Errorf("external_asn.peer_start must be positive")
	}
	if cfg.ExternalASN.CustomerStart <= 0 {
		return fmt.Errorf("external_asn.customer_start must be positive")
	}
	if _, _, err := net.ParseCIDR(cfg.Loopback); err != nil {
		return fmt.Errorf("invalid loopback CIDR %q: %w", cfg.Loopback, err)
	}
	if _, _, err := net.ParseCIDR(cfg.Backbone); err != nil {
		return fmt.Errorf("invalid backbone CIDR %q: %w", cfg.Backbone, err)
	}

	if len(cfg.Sites) == 0 {
		return fmt.Errorf("sites must not be empty")
	}

	siteNames := make(map[string]bool)
	for _, s := range cfg.Sites {
		if s.Name == "" {
			return fmt.Errorf("site name must not be empty")
		}
		if siteNames[s.Name] {
			return fmt.Errorf("duplicate site name: %s", s.Name)
		}
		siteNames[s.Name] = true
	}

	for _, link := range cfg.BackboneLinks {
		if len(link) != 2 {
			return fmt.Errorf("backbone_links entry must have exactly 2 site names")
		}
		for _, name := range link {
			if !siteNames[name] {
				return fmt.Errorf("backbone_links references unknown site: %s", name)
			}
		}
		if link[0] == link[1] {
			return fmt.Errorf("backbone_links entry must not have the same site on both sides: %s", link[0])
		}
	}

	peerNames := make(map[string]bool)
	validTypes := map[string]bool{"transit": true, "peer": true, "customer": true}
	for _, ep := range cfg.ExternalPeers {
		if ep.Name == "" {
			return fmt.Errorf("external_peer name must not be empty")
		}
		if peerNames[ep.Name] {
			return fmt.Errorf("duplicate external_peer name: %s", ep.Name)
		}
		peerNames[ep.Name] = true
		if !validTypes[ep.Type] {
			return fmt.Errorf("external_peer %s has invalid type: %s (must be transit, peer, or customer)", ep.Name, ep.Type)
		}
		if !siteNames[ep.ConnectTo] {
			return fmt.Errorf("external_peer %s references unknown site: %s", ep.Name, ep.ConnectTo)
		}
	}

	return nil
}
