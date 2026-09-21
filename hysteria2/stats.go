package hysteria2

import (
	"net/url"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/sagernet/quic-go"
	"github.com/sagernet/quic-go/congestion"
	"github.com/sagernet/quic-go/monotime"
)

// TransportStats is one side's view of a hysteria2 connection, as a sender.
//
// The counters are cumulative for the connection named by ConnectionID and
// restart from zero on a new connection, so a consumer tracks increments per
// ConnectionID rather than across them.
type TransportStats struct {
	ConnectionID uint64
	SmoothedRTT  time.Duration
	RTTVariance  time.Duration
	// PacketsSent counts the ack-eliciting packets put on the wire, including
	// those later declared lost. Packets that carry only acknowledgements are
	// left out, because they can never be declared lost and would otherwise
	// dilute the loss rate of whichever side mostly receives.
	PacketsSent uint64
	PacketsLost uint64
	BytesSent   uint64
}

// lossCounter counts the packets a connection's congestion controller is told
// were lost.
//
// quic-go keeps its own loss counters only inside its built-in controller, and
// hysteria2 always installs Brutal or BBR in its place, so without this the
// connection statistics report no loss at all.
type lossCounter struct {
	sent atomic.Uint64
	lost atomic.Uint64
}

// countLosses wraps cc so that losses reported to it are counted. The wrapper
// keeps the extended interface when cc has it, because quic-go only delivers
// batched acknowledgements to controllers that implement it.
func countLosses(cc congestion.CongestionControl) (congestion.CongestionControl, *lossCounter) {
	counter := &lossCounter{}
	if extended, isExtended := cc.(congestion.CongestionControlEx); isExtended {
		return &countingControllerEx{CongestionControlEx: extended, counter: counter}, counter
	}
	return &countingController{CongestionControl: cc, counter: counter}, counter
}

type countingController struct {
	congestion.CongestionControl
	counter *lossCounter
}

func (c *countingController) OnPacketSent(sentTime monotime.Time, bytesInFlight congestion.ByteCount, packetNumber congestion.PacketNumber, bytes congestion.ByteCount, isRetransmittable bool) {
	if isRetransmittable {
		c.counter.sent.Add(1)
	}
	c.CongestionControl.OnPacketSent(sentTime, bytesInFlight, packetNumber, bytes, isRetransmittable)
}

func (c *countingController) OnCongestionEvent(number congestion.PacketNumber, lostBytes congestion.ByteCount, priorInFlight congestion.ByteCount) {
	// quic-go reports each lost packet through OnCongestionEvent, whether or
	// not the controller is extended. A zero loss is an ECN congestion signal.
	if lostBytes > 0 {
		c.counter.lost.Add(1)
	}
	c.CongestionControl.OnCongestionEvent(number, lostBytes, priorInFlight)
}

type countingControllerEx struct {
	congestion.CongestionControlEx
	counter *lossCounter
}

func (c *countingControllerEx) OnPacketSent(sentTime monotime.Time, bytesInFlight congestion.ByteCount, packetNumber congestion.PacketNumber, bytes congestion.ByteCount, isRetransmittable bool) {
	if isRetransmittable {
		c.counter.sent.Add(1)
	}
	c.CongestionControlEx.OnPacketSent(sentTime, bytesInFlight, packetNumber, bytes, isRetransmittable)
}

func (c *countingControllerEx) OnCongestionEvent(number congestion.PacketNumber, lostBytes congestion.ByteCount, priorInFlight congestion.ByteCount) {
	if lostBytes > 0 {
		c.counter.lost.Add(1)
	}
	c.CongestionControlEx.OnCongestionEvent(number, lostBytes, priorInFlight)
}

func connectionStats(id uint64, conn *quic.Conn, counter *lossCounter) TransportStats {
	stats := conn.ConnectionStats()
	result := TransportStats{
		ConnectionID: id,
		SmoothedRTT:  stats.SmoothedRTT,
		RTTVariance:  stats.MeanDeviation,
		BytesSent:    stats.BytesSent,
	}
	if counter != nil {
		// Both counts cover the same packets: ack-eliciting ones sent after the
		// controller was installed, which are the only ones reported lost.
		result.PacketsSent = counter.sent.Load()
		result.PacketsLost = counter.lost.Load()
	}
	return result
}

// Stats travel as a URL encoded body, so that either side can add keys later
// without the other having to understand them.
const (
	statsKeySmoothedRTT = "srtt_us"
	statsKeyRTTVariance = "rttvar_us"
	statsKeyPacketsSent = "sent"
	statsKeyPacketsLost = "lost"
	statsKeyBytesSent   = "bytes_sent"
)

func encodeTransportStats(stats TransportStats) string {
	values := url.Values{}
	values.Set(statsKeySmoothedRTT, strconv.FormatInt(stats.SmoothedRTT.Microseconds(), 10))
	values.Set(statsKeyRTTVariance, strconv.FormatInt(stats.RTTVariance.Microseconds(), 10))
	values.Set(statsKeyPacketsSent, strconv.FormatUint(stats.PacketsSent, 10))
	values.Set(statsKeyPacketsLost, strconv.FormatUint(stats.PacketsLost, 10))
	values.Set(statsKeyBytesSent, strconv.FormatUint(stats.BytesSent, 10))
	return values.Encode()
}

func decodeTransportStats(body string) (TransportStats, bool) {
	values, err := url.ParseQuery(body)
	if err != nil {
		return TransportStats{}, false
	}
	var stats TransportStats
	microseconds, err := strconv.ParseInt(values.Get(statsKeySmoothedRTT), 10, 64)
	if err != nil || microseconds < 0 {
		return TransportStats{}, false
	}
	stats.SmoothedRTT = time.Duration(microseconds) * time.Microsecond
	microseconds, err = strconv.ParseInt(values.Get(statsKeyRTTVariance), 10, 64)
	if err != nil || microseconds < 0 {
		return TransportStats{}, false
	}
	stats.RTTVariance = time.Duration(microseconds) * time.Microsecond
	if stats.PacketsSent, err = strconv.ParseUint(values.Get(statsKeyPacketsSent), 10, 64); err != nil {
		return TransportStats{}, false
	}
	if stats.PacketsLost, err = strconv.ParseUint(values.Get(statsKeyPacketsLost), 10, 64); err != nil {
		return TransportStats{}, false
	}
	if stats.BytesSent, err = strconv.ParseUint(values.Get(statsKeyBytesSent), 10, 64); err != nil {
		return TransportStats{}, false
	}
	return stats, true
}
