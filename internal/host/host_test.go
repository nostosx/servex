package host

import (
	"errors"
	"iter"
	"net"
	"net/netip"
	"slices"
	"testing"
)

// fixed returns an address source that yields the given addresses.
func fixed(addrs ...string) func() (iter.Seq[netip.Addr], error) {
	return func() (iter.Seq[netip.Addr], error) {
		parsed := make([]netip.Addr, 0, len(addrs))
		for _, a := range addrs {
			parsed = append(parsed, netip.MustParseAddr(a))
		}
		return slices.Values(parsed), nil
	}
}

func TestExtractPassthrough(t *testing.T) {
	failing := func() (iter.Seq[netip.Addr], error) {
		t.Fatal("address source must not be consulted for a specified host")
		return nil, nil
	}
	tests := []string{
		"10.0.0.1:8000",
		"127.0.0.1:80",
		"[2001:db8::1]:9000",
		"example.com:443",
		"localhost:0",
	}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			got, err := extract(in, failing)
			if err != nil {
				t.Fatalf("extract(%q) error = %v", in, err)
			}
			if got != in {
				t.Fatalf("extract(%q) = %q, want unchanged", in, got)
			}
		})
	}
}

func TestExtractUnspecified(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		addrs []string
		want  string
	}{
		{"empty host", ":8000", []string{"10.0.0.7"}, "10.0.0.7:8000"},
		{"ipv4 any", "0.0.0.0:8000", []string{"10.0.0.7"}, "10.0.0.7:8000"},
		{"ipv6 any", "[::]:9000", []string{"10.0.0.7"}, "10.0.0.7:9000"},
		{"ipv4 preferred over earlier ipv6", ":1", []string{"2001:db8::5", "192.168.1.2"}, "192.168.1.2:1"},
		{"ipv6 fallback", ":2", []string{"fe80::1", "2001:db8::5", "2001:db8::6"}, "[2001:db8::5]:2"},
		{"skips loopback and link-local", ":3", []string{"127.0.0.1", "::1", "169.254.0.9", "fe80::2", "172.16.0.3"}, "172.16.0.3:3"},
		{"ipv4-mapped is unmapped", ":4", []string{"::ffff:10.1.2.3"}, "10.1.2.3:4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extract(tt.in, fixed(tt.addrs...))
			if err != nil {
				t.Fatalf("extract(%q) error = %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("extract(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestExtractErrors(t *testing.T) {
	sourceErr := errors.New("boom")
	tests := []struct {
		name   string
		in     string
		source func() (iter.Seq[netip.Addr], error)
		want   error
	}{
		{"missing port", "10.0.0.1", fixed(), nil},
		{"no usable address", ":80", fixed("127.0.0.1", "::1"), errNoAddress},
		{"source failure", "0.0.0.0:80", func() (iter.Seq[netip.Addr], error) { return nil, sourceErr }, sourceErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extract(tt.in, tt.source)
			if err == nil {
				t.Fatalf("extract(%q) error = nil, want error", tt.in)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("extract(%q) error = %v, want %v", tt.in, err, tt.want)
			}
		})
	}
}

// TestExtractInterfaces exercises the real interface scan. The result
// depends on the machine, so only its shape is checked.
func TestExtractInterfaces(t *testing.T) {
	got, err := Extract("0.0.0.0:8080")
	if errors.Is(err, errNoAddress) {
		t.Skip("no non-loopback address on this machine")
	}
	if err != nil {
		t.Fatalf("Extract error = %v", err)
	}
	h, port, err := net.SplitHostPort(got)
	if err != nil {
		t.Fatalf("Extract returned %q: %v", got, err)
	}
	if port != "8080" {
		t.Fatalf("port = %q, want 8080", port)
	}
	a, err := netip.ParseAddr(h)
	if err != nil {
		t.Fatalf("host %q is not an IP: %v", h, err)
	}
	if !usable(a) {
		t.Fatalf("host %q is not a usable address", h)
	}
}
