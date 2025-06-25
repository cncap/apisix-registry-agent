package apisixregistryagent

import (
	"testing"
)

func TestBuildUpstream_Static(t *testing.T) {
	opts := Options{
		Env:          "dev",
		UseDiscovery: false,
		ServiceID:    "test-service",
		Port:         8082,
		StaticNodes:  map[string]int{"test-service:8082": 1},
	}
	up, err := BuildUpstream(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if up["nodes"] == nil {
		t.Errorf("expected nodes in upstream")
	}
}

func TestBuildUpstream_Discovery(t *testing.T) {
	opts := Options{
		Env:           "prod",
		UseDiscovery:  true,
		DiscoveryType: "kubernetes",
		ServiceID:     "test-service",
	}
	up, err := BuildUpstream(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if up["discovery_type"] != "kubernetes" {
		t.Errorf("expected discovery_type kubernetes, got %v", up["discovery_type"])
	}
	if up["service_name"] != "test-service.default.svc.cluster.local" {
		t.Errorf("unexpected service_name: %v", up["service_name"])
	}
}
