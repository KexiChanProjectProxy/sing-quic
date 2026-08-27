package realm

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
)

const (
	maxIPv6LookupBody        = 8 << 10
	ipv6LookupUA             = "sing-box"
	defaultIPv6LookupTimeout = 4 * time.Second
)

type IPv6LookupConfig struct {
	URL       string
	LocalPort uint16
	Timeout   time.Duration
	Client    *http.Client
}

func NormalizeIPv6LookupURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", E.Cause(err, "invalid ipv6_api")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", E.New("ipv6_api must be an http or https URL")
	}
	if u.Host == "" {
		return "", E.New("ipv6_api host is required")
	}
	return raw, nil
}

func LookupIPv6(ctx context.Context, config IPv6LookupConfig) ([]netip.AddrPort, error) {
	endpoint, err := NormalizeIPv6LookupURL(config.URL)
	if err != nil {
		return nil, err
	}
	if endpoint == "" {
		return nil, E.New("ipv6_api URL is required")
	}
	if config.LocalPort == 0 {
		return nil, E.New("local listen port is required")
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = defaultIPv6LookupTimeout
	}
	if timeout < 0 {
		return nil, E.New("ipv6_api timeout must not be negative")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := config.Client
	if client == nil {
		client = ipv6OnlyHTTPClient(timeout)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ipv6LookupUA)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, E.New("ipv6_api HTTP ", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxIPv6LookupBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxIPv6LookupBody {
		return nil, E.New("ipv6_api response too large")
	}
	return parseIPv6LookupBody(body, config.LocalPort)
}

func SupplementIPv6(ctx context.Context, addrs []netip.AddrPort, apiURL string, localPort uint16, log logger.Logger) []netip.AddrPort {
	if strings.TrimSpace(apiURL) == "" || localPort == 0 {
		return addrs
	}
	extra, err := LookupIPv6(ctx, IPv6LookupConfig{URL: apiURL, LocalPort: localPort})
	if err != nil {
		if log != nil {
			log.Warn("realm IPv6 HTTP lookup failed; continuing with STUN addresses: ", err)
		}
		return addrs
	}
	return InsertAddrPorts(addrs, extra)
}

func ipv6OnlyHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: timeout}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = func(ctx context.Context, _, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp6", addr)
	}
	return &http.Client{Timeout: timeout, Transport: tr}
}

func parseIPv6LookupBody(body []byte, localPort uint16) ([]netip.AddrPort, error) {
	s := strings.TrimSpace(string(body))
	if s == "" {
		return nil, E.New("empty ipv6_api response")
	}
	if addrs, err := parseIPv6LookupText(s, localPort); err == nil {
		return addrs, nil
	}
	var raw any
	if err := json.Unmarshal(body, &raw); err == nil {
		if addrs := ipv6AddrPortsFromJSON(raw, localPort); len(addrs) > 0 {
			return InsertAddrPorts(nil, addrs), nil
		}
	}
	if addrs := ipv6AddrPortsFromTrace(s, localPort); len(addrs) > 0 {
		return InsertAddrPorts(nil, addrs), nil
	}
	return nil, E.New("no IPv6 address in ipv6_api response")
}

func parseIPv6LookupText(s string, localPort uint16) ([]netip.AddrPort, error) {
	if ap, ok := parseIPv6Token(s, localPort); ok {
		return []netip.AddrPort{ap}, nil
	}
	var out []netip.AddrPort
	for _, line := range strings.Split(s, "\n") {
		if ap, ok := parseIPv6Token(line, localPort); ok {
			out = append(out, ap)
		}
	}
	if len(out) == 0 {
		return nil, E.New("no IPv6 address in ipv6_api response")
	}
	return InsertAddrPorts(nil, out), nil
}

func ipv6AddrPortsFromJSON(raw any, localPort uint16) []netip.AddrPort {
	switch v := raw.(type) {
	case string:
		if ap, ok := parseIPv6Token(v, localPort); ok {
			return []netip.AddrPort{ap}
		}
	case []any:
		var out []netip.AddrPort
		for _, item := range v {
			out = append(out, ipv6AddrPortsFromJSON(item, localPort)...)
		}
		return out
	case map[string]any:
		var out []netip.AddrPort
		for _, key := range []string{"ip", "ipv6", "address", "origin", "query"} {
			if item, ok := v[key]; ok {
				out = append(out, ipv6AddrPortsFromJSON(item, localPort)...)
			}
		}
		return out
	}
	return nil
}

func ipv6AddrPortsFromTrace(s string, localPort uint16) []netip.AddrPort {
	var out []netip.AddrPort
	for _, line := range strings.Split(s, "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || !strings.EqualFold(key, "ip") {
			continue
		}
		if ap, ok := parseIPv6Token(val, localPort); ok {
			out = append(out, ap)
		}
	}
	return out
}

func parseIPv6Token(s string, localPort uint16) (netip.AddrPort, bool) {
	s = strings.Trim(strings.TrimSpace(s), `"'`)
	if s == "" || localPort == 0 {
		return netip.AddrPort{}, false
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		if usableLookupIPv6(ap.Addr()) {
			return netip.AddrPortFrom(ap.Addr().Unmap(), localPort), true
		}
		return netip.AddrPort{}, false
	}
	ip, err := netip.ParseAddr(s)
	if err != nil || !usableLookupIPv6(ip) {
		return netip.AddrPort{}, false
	}
	return netip.AddrPortFrom(ip.Unmap(), localPort), true
}

func usableLookupIPv6(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.Is6() &&
		!addr.IsLoopback() &&
		!addr.IsUnspecified() &&
		!addr.IsMulticast() &&
		!addr.IsLinkLocalUnicast()
}
func PacketConnPort(addr net.Addr) uint16 {
	if u, ok := addr.(*net.UDPAddr); ok && u.Port > 0 && u.Port <= 65535 {
		return uint16(u.Port)
	}
	return 0
}
