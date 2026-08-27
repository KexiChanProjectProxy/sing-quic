package realm

import (
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"

	"github.com/sagernet/sing-quic/hysteria"
	E "github.com/sagernet/sing/common/exceptions"
)

func ParseListenPorts(specs []string) ([]uint16, error) {
	var out []uint16
	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		if spec == "*" || spec == "all" {
			return nil, E.New("listen_ports ", spec, " is not supported")
		}
		spec = strings.ReplaceAll(spec, "-", ":")
		if !strings.Contains(spec, ":") {
			port, err := strconv.ParseUint(spec, 10, 16)
			if err != nil || port == 0 {
				return nil, E.New("invalid listen_ports: ", spec)
			}
			out = append(out, uint16(port))
			continue
		}
		ports, err := hysteria.ParsePorts([]string{spec})
		if err != nil {
			return nil, E.Cause(err, "invalid listen_ports: ", spec)
		}
		for _, port := range ports {
			if port != 0 {
				out = append(out, port)
			}
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

func ShuffleListenPorts(ports []uint16) []uint16 {
	out := slices.Clone(ports)
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}
