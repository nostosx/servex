package host

import (
	"errors"
	"fmt"
	"iter"
	"net"
	"net/netip"
)

// errNoAddress is returned when no usable non-loopback address exists.
var errNoAddress = errors.New("host: no non-loopback address found")

// Extract converts a listen address into a host:port that can be registered
// for external use. When the host is empty, 0.0.0.0 or ::, it is replaced by
// the first usable IPv4 address of this machine, falling back to IPv6. Loopback,
// unspecified, link-local and multicast addresses are excluded from that search.
// Explicit hosts are returned unchanged. Choosing a local address does not
// verify reachability from peers.
func Extract(hostport string) (string, error) {
	return extract(hostport, interfaceAddrs)
}

// extract implements Extract with an injectable address source.
func extract(hostport string, addrs func() (iter.Seq[netip.Addr], error)) (string, error) {
	h, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return "", fmt.Errorf("host: invalid address %q: %w", hostport, err)
	}
	if !unspecified(h) {
		return hostport, nil
	}
	seq, err := addrs()
	if err != nil {
		return "", fmt.Errorf("host: list interface addresses: %w", err)
	}
	ip, ok := pick(seq)
	if !ok {
		return "", errNoAddress
	}
	return net.JoinHostPort(ip.String(), port), nil
}

// unspecified reports whether h denotes "any address".
func unspecified(h string) bool {
	if h == "" {
		return true
	}
	a, err := netip.ParseAddr(h)
	return err == nil && a.IsUnspecified()
}

// pick returns the first usable IPv4 address in addrs, falling back to the
// first usable IPv6 address.
func pick(addrs iter.Seq[netip.Addr]) (netip.Addr, bool) {
	var v6 netip.Addr
	for a := range addrs {
		a = a.Unmap()
		if !usable(a) {
			continue
		}
		if a.Is4() {
			return a, true
		}
		if !v6.IsValid() {
			v6 = a
		}
	}
	return v6, v6.IsValid()
}

// usable reports whether a is a candidate for automatic address selection.
func usable(a netip.Addr) bool {
	return a.IsValid() &&
		!a.IsLoopback() &&
		!a.IsUnspecified() &&
		!a.IsLinkLocalUnicast() &&
		!a.IsMulticast()
}

// interfaceAddrs yields the unicast addresses of all up, non-loopback
// interfaces in interface index order.
func interfaceAddrs() (iter.Seq[netip.Addr], error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	return func(yield func(netip.Addr) bool) {
		for _, ifc := range ifaces {
			if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := ifc.Addrs()
			if err != nil {
				continue
			}
			for _, a := range addrs {
				var ip net.IP
				switch v := a.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				default:
					continue
				}
				addr, ok := netip.AddrFromSlice(ip)
				if !ok {
					continue
				}
				if !yield(addr.Unmap()) {
					return
				}
			}
		}
	}, nil
}
