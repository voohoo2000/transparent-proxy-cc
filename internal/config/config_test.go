package config

import (
	"testing"
)

func TestFromEnvValidLANMode(t *testing.T) {
	env := map[string]string{
		"MODE":                "lan-gateway",
		"UPSTREAM_PROXY_HOST": "proxy.company.local",
		"UPSTREAM_PROXY_PORT": "8080",
		"LISTEN_PORT":        "12345",
		"LAN_INTERFACE":      "eth0",
		"EXCLUDE_CIDRS":      "10.0.0.0/8,192.168.0.0/16",
		"LOG_LEVEL":          "debug",
	}

	cfg, err := FromMap(env)
	if err != nil {
		t.Fatalf("FromMap returned error: %v", err)
	}
	if cfg.Mode != ModeLANGateway {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, ModeLANGateway)
	}
	if cfg.UpstreamProxyHost != "proxy.company.local" {
		t.Fatalf("UpstreamProxyHost = %q", cfg.UpstreamProxyHost)
	}
	if cfg.UpstreamProxyPort != 8080 {
		t.Fatalf("UpstreamProxyPort = %d", cfg.UpstreamProxyPort)
	}
	if cfg.ListenPort != 12345 {
		t.Fatalf("ListenPort = %d", cfg.ListenPort)
	}
	if cfg.LANInterface != "eth0" {
		t.Fatalf("LANInterface = %q", cfg.LANInterface)
	}
	if len(cfg.ExcludeCIDRs) != 2 {
		t.Fatalf("ExcludeCIDRs length = %d", len(cfg.ExcludeCIDRs))
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q", cfg.LogLevel)
	}
}

func TestFromEnvDefaults(t *testing.T) {
	env := map[string]string{
		"UPSTREAM_PROXY_HOST": "proxy.company.local",
		"UPSTREAM_PROXY_PORT": "8080",
	}

	cfg, err := FromMap(env)
	if err != nil {
		t.Fatalf("FromMap returned error: %v", err)
	}
	if cfg.Mode != ModeLANGateway {
		t.Fatalf("Mode = %q, want default %q", cfg.Mode, ModeLANGateway)
	}
	if cfg.ListenPort != 12345 {
		t.Fatalf("ListenPort = %d, want 12345", cfg.ListenPort)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want info", cfg.LogLevel)
	}
}

func TestFromEnvRejectsInvalidMode(t *testing.T) {
	env := map[string]string{
		"MODE":                "bad",
		"UPSTREAM_PROXY_HOST": "proxy.company.local",
		"UPSTREAM_PROXY_PORT": "8080",
	}

	_, err := FromMap(env)
	if err == nil {
		t.Fatal("FromMap returned nil error for invalid mode")
	}
}

func TestFromEnvRejectsMissingUpstreamHost(t *testing.T) {
	env := map[string]string{
		"UPSTREAM_PROXY_PORT": "8080",
	}

	_, err := FromMap(env)
	if err == nil {
		t.Fatal("FromMap returned nil error for missing upstream host")
	}
}

func TestFromEnvRejectsInvalidPorts(t *testing.T) {
	env := map[string]string{
		"UPSTREAM_PROXY_HOST": "proxy.company.local",
		"UPSTREAM_PROXY_PORT": "70000",
	}

	_, err := FromMap(env)
	if err == nil {
		t.Fatal("FromMap returned nil error for invalid upstream port")
	}
}
