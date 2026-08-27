package realm

import (
	"net/netip"
	"testing"
	"time"
)

func TestParsePreferIPVersion(t *testing.T) {
	t.Parallel()
	got, err := ParsePreferIPVersion("")
	if err != nil || got != PreferIPVersion6 {
		t.Fatalf("empty: got %v %v", got, err)
	}
	got, err = ParsePreferIPVersion("v4")
	if err != nil || got != PreferIPVersion4 {
		t.Fatalf("v4: got %v %v", got, err)
	}
	got, err = ParsePreferIPVersion("dual")
	if err != nil || got != PreferIPVersionBoth {
		t.Fatalf("dual: got %v %v", got, err)
	}
	if _, err = ParsePreferIPVersion("bogus"); err == nil {
		t.Fatal("expected error")
	}
}

func TestPunchSchedulePrefersIPv6UntilTimeout(t *testing.T) {
	t.Parallel()
	v6 := netip.MustParseAddrPort("[2001:db8::1]:4433")
	v4 := netip.MustParseAddrPort("192.0.2.1:4433")
	now := time.Unix(1000, 0)
	s := newPunchSchedule([]netip.AddrPort{v4, v6}, 2*time.Second, now, PreferIPVersion6)
	if got := s.addrs(now); len(got) != 1 || got[0] != v6 {
		t.Fatalf("before fallback: %v", got)
	}
	if got := s.addrs(now.Add(2 * time.Second)); len(got) != 2 || got[0] != v6 || got[1] != v4 {
		t.Fatalf("after fallback: %v", got)
	}
}

func TestPunchSchedulePrefersIPv4UntilTimeout(t *testing.T) {
	t.Parallel()
	v6 := netip.MustParseAddrPort("[2001:db8::1]:4433")
	v4 := netip.MustParseAddrPort("192.0.2.1:4433")
	now := time.Unix(1000, 0)
	s := newPunchSchedule([]netip.AddrPort{v4, v6}, 2*time.Second, now, PreferIPVersion4)
	if got := s.addrs(now); len(got) != 1 || got[0] != v4 {
		t.Fatalf("before fallback: %v", got)
	}
}

func TestPunchOptionsTimingExtendsWhenFallbackLong(t *testing.T) {
	t.Parallel()
	timeout, fallback := PunchOptions{FallbackTimeout: 10 * time.Second}.Timing()
	if fallback != 10*time.Second {
		t.Fatalf("fallback %v", fallback)
	}
	if timeout != 20*time.Second {
		t.Fatalf("timeout %v", timeout)
	}
}

func TestParseListenPorts(t *testing.T) {
	t.Parallel()
	got, err := ParseListenPorts([]string{"60000-61000"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1001 || got[0] != 60000 || got[len(got)-1] != 61000 {
		t.Fatalf("range: len=%d first=%d last=%d", len(got), got[0], got[len(got)-1])
	}
	got, err = ParseListenPorts([]string{"443"})
	if err != nil || len(got) != 1 || got[0] != 443 {
		t.Fatalf("single: %v %v", got, err)
	}
	if _, err = ParseListenPorts([]string{"*"}); err == nil {
		t.Fatal("expected error for *")
	}
}
