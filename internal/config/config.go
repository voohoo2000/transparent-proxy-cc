package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const (
	ModeLANGateway = "lan-gateway"
	ModeLocal      = "local"
)

type Config struct {
	Mode              string
	UpstreamProxyHost string
	UpstreamProxyPort int
	ListenPort        int
	LANInterface      string
	ExcludeCIDRs      []*net.IPNet
	LogLevel          string
}

func FromEnv() (Config, error) {
	return FromMap(map[string]string{
		"MODE":                os.Getenv("MODE"),
		"UPSTREAM_PROXY_HOST": os.Getenv("UPSTREAM_PROXY_HOST"),
		"UPSTREAM_PROXY_PORT": os.Getenv("UPSTREAM_PROXY_PORT"),
		"LISTEN_PORT":        os.Getenv("LISTEN_PORT"),
		"LAN_INTERFACE":      os.Getenv("LAN_INTERFACE"),
		"EXCLUDE_CIDRS":      os.Getenv("EXCLUDE_CIDRS"),
		"LOG_LEVEL":          os.Getenv("LOG_LEVEL"),
	})
}

func FromMap(env map[string]string) (Config, error) {
	cfg := Config{
		Mode:         valueOrDefault(env["MODE"], ModeLANGateway),
		ListenPort:   12345,
		LANInterface: env["LAN_INTERFACE"],
		LogLevel:     valueOrDefault(env["LOG_LEVEL"], "info"),
	}

	if cfg.Mode != ModeLANGateway && cfg.Mode != ModeLocal {
		return Config{}, fmt.Errorf("invalid MODE %q", cfg.Mode)
	}

	cfg.UpstreamProxyHost = strings.TrimSpace(env["UPSTREAM_PROXY_HOST"])
	if cfg.UpstreamProxyHost == "" {
		return Config{}, fmt.Errorf("UPSTREAM_PROXY_HOST is required")
	}

	upstreamPort, err := parsePort(env["UPSTREAM_PROXY_PORT"], "UPSTREAM_PROXY_PORT")
	if err != nil {
		return Config{}, err
	}
	cfg.UpstreamProxyPort = upstreamPort

	if strings.TrimSpace(env["LISTEN_PORT"]) != "" {
		listenPort, err := parsePort(env["LISTEN_PORT"], "LISTEN_PORT")
		if err != nil {
			return Config{}, err
		}
		cfg.ListenPort = listenPort
	}

	cidrs, err := parseCIDRs(env["EXCLUDE_CIDRS"])
	if err != nil {
		return Config{}, err
	}
	cfg.ExcludeCIDRs = cidrs

	if cfg.LogLevel != "info" && cfg.LogLevel != "debug" {
		return Config{}, fmt.Errorf("invalid LOG_LEVEL %q", cfg.LogLevel)
	}

	return cfg, nil
}

func valueOrDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func parsePort(value string, name string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("%s is required", name)
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s must be a port from 1 to 65535", name)
	}
	return port, nil
}

func parseCIDRs(value string) ([]*net.IPNet, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.0/8"
	}

	parts := strings.Split(value, ",")
	cidrs := make([]*net.IPNet, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		_, ipNet, err := net.ParseCIDR(part)
		if err != nil {
			return nil, fmt.Errorf("invalid EXCLUDE_CIDRS entry %q: %w", part, err)
		}
		cidrs = append(cidrs, ipNet)
	}
	return cidrs, nil
}
