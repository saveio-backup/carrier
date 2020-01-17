package metrics

type StreamMeterCallback func(int64, string, string)
type MeterCallback func(int64)

type Reporter interface {
	LogSentMessage(int64)
	LogRecvMessage(int64)
	LogSentMessageStream(int64, string, string)
	LogRecvMessageStream(int64, string, string)
	GetBandwidthForPeer(string) Stats
	GetBandwidthForProtocol(string) Stats
	GetBandwidthTotals() Stats
}
