package realm

import (
	"strings"
	"time"

	E "github.com/sagernet/sing/common/exceptions"
)

type PreferIPVersion int

const (
	PreferIPVersion6 PreferIPVersion = iota
	PreferIPVersion4
	PreferIPVersionBoth
)

const defaultPunchFallbackTimeout = 2 * time.Second

type PunchOptions struct {
	Timeout         time.Duration
	FallbackTimeout time.Duration
	Prefer          PreferIPVersion
}

func ParsePreferIPVersion(value string) (PreferIPVersion, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "v6", "6":
		return PreferIPVersion6, nil
	case "v4", "4":
		return PreferIPVersion4, nil
	case "dual", "both":
		return PreferIPVersionBoth, nil
	default:
		return 0, E.New("invalid prefer_ip_version: ", value)
	}
}

func (o PunchOptions) Timing() (timeout, fallback time.Duration) {
	fallback = o.FallbackTimeout
	if fallback <= 0 {
		fallback = defaultPunchFallbackTimeout
	}
	timeout = o.Timeout
	if timeout <= 0 {
		timeout = punchTimeout
		if fallback >= timeout {
			timeout = fallback + punchTimeout
		}
	}
	return timeout, fallback
}
