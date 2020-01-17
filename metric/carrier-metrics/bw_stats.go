package metrics

import (
	"github.com/saveio/carrier/metric/go-flow-metrics"
)

type Stats struct {
	TotalIn  int64
	TotalOut int64
	RateIn   float64
	RateOut  float64
}

type BandwidthCounter struct {
	totalIn  flow.Meter
	totalOut flow.Meter

	protocolIn  flow.MeterRegistry
	protocolOut flow.MeterRegistry

	peerIn  flow.MeterRegistry
	peerOut flow.MeterRegistry
}

func NewBandwidthCounter() *BandwidthCounter {
	return new(BandwidthCounter)
}

func (bwc *BandwidthCounter) LogSentMessage(size int64) {
	bwc.totalOut.Mark(uint64(size))
}

func (bwc *BandwidthCounter) LogRecvMessage(size int64) {
	bwc.totalIn.Mark(uint64(size))
}

func (bwc *BandwidthCounter) LogSentMessageConnOnly(size int64, proto string) {
	bwc.protocolOut.Get(string(proto)).Mark(uint64(size))
}

func (bwc *BandwidthCounter) LogSentMessageStream(size int64, proto, p string) {
	bwc.protocolOut.Get(string(proto)).Mark(uint64(size))
	bwc.peerOut.Get(string(p)).Mark(uint64(size))
}

func (bwc *BandwidthCounter) LogRecvMessageStream(size int64, proto, p string) {
	bwc.protocolIn.Get(string(proto)).Mark(uint64(size))
	bwc.peerIn.Get(string(p)).Mark(uint64(size))
}

func (bwc *BandwidthCounter) GetBandwidthForPeer(p string) (out Stats) {
	inSnap := bwc.peerIn.Get(string(p)).Snapshot()
	outSnap := bwc.peerOut.Get(string(p)).Snapshot()

	return Stats{
		TotalIn:  int64(inSnap.Total),
		TotalOut: int64(outSnap.Total),
		RateIn:   inSnap.Rate,
		RateOut:  outSnap.Rate,
	}
}

func (bwc *BandwidthCounter) GetBandwidthForProtocol(proto string) (out Stats) {
	inSnap := bwc.protocolIn.Get(string(proto)).Snapshot()
	outSnap := bwc.protocolOut.Get(string(proto)).Snapshot()

	return Stats{
		TotalIn:  int64(inSnap.Total),
		TotalOut: int64(outSnap.Total),
		RateIn:   inSnap.Rate,
		RateOut:  outSnap.Rate,
	}
}

func (bwc *BandwidthCounter) GetBandwidthTotals() (out Stats) {
	inSnap := bwc.totalIn.Snapshot()
	outSnap := bwc.totalOut.Snapshot()

	return Stats{
		TotalIn:  int64(inSnap.Total),
		TotalOut: int64(outSnap.Total),
		RateIn:   inSnap.Rate,
		RateOut:  outSnap.Rate,
	}
}
