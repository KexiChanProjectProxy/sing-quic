package hysteria2

import (
	"testing"
	"time"

	"github.com/sagernet/quic-go/congestion"
	congestion_meta2 "github.com/sagernet/sing-quic/congestion_meta2"
	hyCC "github.com/sagernet/sing-quic/hysteria/congestion"
)

func TestTransportStatsRoundTrip(t *testing.T) {
	stats := TransportStats{
		SmoothedRTT: 42 * time.Millisecond,
		RTTVariance: 3 * time.Millisecond,
		PacketsSent: 1000,
		PacketsLost: 17,
		BytesSent:   1 << 30,
	}
	decoded, ok := decodeTransportStats(encodeTransportStats(stats))
	if !ok {
		t.Fatal("decode failed")
	}
	if decoded != stats {
		t.Fatalf("decoded %+v, want %+v", decoded, stats)
	}
}

func TestTransportStatsDecodeIgnoresUnknownKeys(t *testing.T) {
	body := encodeTransportStats(TransportStats{PacketsSent: 5}) + "&future_key=1"
	decoded, ok := decodeTransportStats(body)
	if !ok || decoded.PacketsSent != 5 {
		t.Fatalf("decoded %+v ok=%v", decoded, ok)
	}
}

func TestTransportStatsDecodeRejectsMalformed(t *testing.T) {
	for _, body := range []string{
		"",
		"srtt_us=1",
		"srtt_us=-1&rttvar_us=0&sent=0&lost=0&bytes_sent=0",
		"srtt_us=x&rttvar_us=0&sent=0&lost=0&bytes_sent=0",
		"srtt_us=1&rttvar_us=0&sent=-5&lost=0&bytes_sent=0",
		"%zz",
	} {
		if _, ok := decodeTransportStats(body); ok {
			t.Errorf("accepted %q", body)
		}
	}
}

// quic-go only delivers batched acknowledgements to extended controllers, so
// wrapping must not hide that interface.
func TestCountLossesKeepsExtendedInterface(t *testing.T) {
	for name, cc := range map[string]congestion.CongestionControl{
		"brutal": hyCC.NewBrutalSender(1<<20, 1200, false, nil),
		"bbr":    congestion_meta2.NewBbrSenderWithProfile(1200, congestion_meta2.ProfileStandard),
	} {
		wrapped, _ := countLosses(cc)
		if _, isExtended := wrapped.(congestion.CongestionControlEx); !isExtended {
			t.Errorf("%s lost the extended interface when wrapped", name)
		}
	}
}

type plainController struct {
	congestion.CongestionControl
	events int
}

func (p *plainController) OnCongestionEvent(congestion.PacketNumber, congestion.ByteCount, congestion.ByteCount) {
	p.events++
}

func TestCountLossesCountsOnlyLosses(t *testing.T) {
	inner := &plainController{}
	wrapped, counter := countLosses(inner)
	wrapped.OnCongestionEvent(1, 1200, 0)
	wrapped.OnCongestionEvent(2, 0, 0) // an ECN signal, not a loss
	wrapped.OnCongestionEvent(3, 800, 0)
	if got := counter.lost.Load(); got != 2 {
		t.Fatalf("counted %d losses, want 2", got)
	}
	if inner.events != 3 {
		t.Fatalf("inner controller saw %d events, want every one", inner.events)
	}
}
