package main

import (
	"testing"
)

func TestParseHosts_單一地址(t *testing.T) {
	hosts, err := parseHosts("127.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 1 || hosts[0] != "127.0.0.1" {
		t.Fatalf("got %v, want [127.0.0.1]", hosts)
	}
}

func TestParseHosts_多個地址(t *testing.T) {
	hosts, err := parseHosts("127.0.0.1,192.168.1.10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("got %d hosts, want 2", len(hosts))
	}
	if hosts[0] != "127.0.0.1" || hosts[1] != "192.168.1.10" {
		t.Fatalf("got %v, want [127.0.0.1 192.168.1.10]", hosts)
	}
}

func TestParseHosts_去重(t *testing.T) {
	hosts, err := parseHosts("127.0.0.1,127.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("got %d hosts, want 1 (deduped)", len(hosts))
	}
}

func TestParseHosts_含空白(t *testing.T) {
	hosts, err := parseHosts(" 127.0.0.1 , 192.168.1.10 ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("got %d hosts, want 2", len(hosts))
	}
}

func TestParseHosts_無效地址(t *testing.T) {
	_, err := parseHosts("not-an-ip")
	if err == nil {
		t.Fatal("expected error for invalid address")
	}
}

func TestParseHosts_空字串(t *testing.T) {
	_, err := parseHosts("")
	if err == nil {
		t.Fatal("expected error for empty string")
	}
}

func TestParseHosts_IPv6(t *testing.T) {
	hosts, err := parseHosts("::1,127.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 2 || hosts[0] != "::1" {
		t.Fatalf("got %v, want [::1 127.0.0.1]", hosts)
	}
}

func TestParseHosts_0000(t *testing.T) {
	hosts, err := parseHosts("0.0.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 1 || hosts[0] != "0.0.0.0" {
		t.Fatalf("got %v, want [0.0.0.0]", hosts)
	}
}
