package realm

import (
	"net/netip"
	"slices"
	"strings"
)

func InsertAddrPort(addrs []netip.AddrPort, addr netip.AddrPort) []netip.AddrPort {
	if !addr.IsValid() || addr.Port() == 0 {
		return addrs
	}
	out := append([]netip.AddrPort(nil), addrs...)
	i, found := slices.BinarySearchFunc(out, addr, func(a, b netip.AddrPort) int {
		return strings.Compare(a.String(), b.String())
	})
	if found {
		return out
	}
	return slices.Insert(out, i, addr)
}

func InsertAddrPorts(addrs []netip.AddrPort, extra []netip.AddrPort) []netip.AddrPort {
	for _, addr := range extra {
		addrs = InsertAddrPort(addrs, addr)
	}
	return addrs
}
