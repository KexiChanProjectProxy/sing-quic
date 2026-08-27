package realm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestParseIPv6LookupBody(t *testing.T) {
	t.Parallel()
	want := netip.MustParseAddrPort("[2001:db8::1]:4433")
	tests := []struct {
		name string
		body string
		want []netip.AddrPort
	}{
		{name: "plain", body: "2001:db8::1\n", want: []netip.AddrPort{want}},
		{name: "addrport uses listen port", body: "[2001:db8::1]:9999", want: []netip.AddrPort{want}},
		{name: "json object", body: `{"ip":"2001:db8::1"}`, want: []netip.AddrPort{want}},
		{name: "trace", body: "fl=x\nip=2001:db8::1\nts=1\n", want: []netip.AddrPort{want}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseIPv6LookupBody([]byte(tc.body), 4433)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) || got[0] != tc.want[0] {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestParseIPv6LookupBodyRejectsIPv4(t *testing.T) {
	t.Parallel()
	if _, err := parseIPv6LookupBody([]byte("192.0.2.1\n"), 4433); err == nil {
		t.Fatal("expected error")
	}
}

func TestLookupIPv6HTTPIgnoresResponsePort(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[2001:db8::aa]:9999\n"))
	}))
	t.Cleanup(ts.Close)
	addrs, err := LookupIPv6(context.Background(), IPv6LookupConfig{
		URL:       ts.URL,
		LocalPort: 4433,
		Client:    ts.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := netip.MustParseAddrPort("[2001:db8::aa]:4433")
	if len(addrs) != 1 || addrs[0] != want {
		t.Fatalf("got %v", addrs)
	}
}

func TestLookupIPv6RequiresLocalPort(t *testing.T) {
	t.Parallel()
	if _, err := LookupIPv6(context.Background(), IPv6LookupConfig{URL: "https://api6.ipify.org"}); err == nil {
		t.Fatal("expected error")
	}
}
