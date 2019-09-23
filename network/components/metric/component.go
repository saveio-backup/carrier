/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-09-12
 */
package metric

import (
	"context"
	"math/rand"

	"sync"

	"time"

	"math"

	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/network"
	"github.com/saveio/themis/common/log"
)

type TimeDistance struct {
	startTime int64
	endTime   int64
	distance  int64
}

type TimeMetric struct {
	timeMetric []TimeDistance
	index      int
	mutex      *sync.Mutex
	isFull     bool
}

type MetricComponent struct {
	*network.Component
	net            *network.Network
	sampleInterval time.Duration
	sampleSize     int
	packageSize    int
	sampleData     TimeMetric
	randRaw        []byte
}

// ComponentOption are configurable options for the backoff Component
type ComponentOption func(*MetricComponent)

// WithInitialDelay specifies initial backoff interval
func WithSampleInterval(d time.Duration) ComponentOption {
	return func(o *MetricComponent) {
		o.sampleInterval = d
	}
}

func WithSampleSize(size int) ComponentOption {
	return func(o *MetricComponent) {
		o.sampleSize = size
	}
}

func WithPackageSize(size int) ComponentOption {
	return func(o *MetricComponent) {
		o.packageSize = size
	}
}

const (
	defaultSampleInterval = 3 * time.Second
	defaultSampleSize     = 100
	defaultPackageSize    = 1024 * 16
)

func defaultOptions() ComponentOption {
	return func(o *MetricComponent) {
		o.sampleInterval = defaultSampleInterval
		o.sampleSize = defaultSampleSize
		o.packageSize = defaultPackageSize
	}
}

var (
	_ network.ComponentInterface = (*MetricComponent)(nil)
)

// New returns a new backoff Component with specified options
func New(opts ...ComponentOption) *MetricComponent {
	p := new(MetricComponent)
	defaultOptions()(p)
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *MetricComponent) Startup(n *network.Network) {
	p.net = n
	p.sampleData = TimeMetric{
		timeMetric: make([]TimeDistance, p.sampleSize),
		index:      0,
		isFull:     false,
		mutex:      new(sync.Mutex)}

	p.randRaw = getRawData(p.packageSize)
}

func getRawData(bytes int) []byte {
	var raw []byte
	for i := 0; i < bytes; i++ {
		raw = append(raw, byte(rand.Int()/128))
	}
	return raw
}

type element struct {
	distance int64
	count    int64
	rate     float64
}

func (p *MetricComponent) timeDistanceExpect() int64 {
	if p.sampleData.index == 0 {
		return 0
	}

	statics := make([]element, p.sampleData.index)
	var elemCnt int
	for i := 0; i < p.sampleData.index; i++ {
		var distance int64
		var hasExist bool
		if p.sampleData.timeMetric[i].endTime == 0 {
			distance = time.Now().UnixNano()/int64(time.Microsecond) - p.sampleData.timeMetric[i].startTime
		} else {
			distance = p.sampleData.timeMetric[i].endTime - p.sampleData.timeMetric[i].startTime
		}
		p.sampleData.timeMetric[i].distance = distance

		for j := 0; j < elemCnt; j++ {
			if distance == statics[j].distance {
				statics[j].count++
				hasExist = true
				break
			}
		}

		if hasExist == false {
			statics[elemCnt].distance = distance
			statics[elemCnt].count = 1
			elemCnt++
		}
	}

	var sum float64
	for i := 0; i < elemCnt; i++ {
		statics[i].rate = float64(statics[i].count) / float64(p.sampleData.index)
		sum += statics[i].rate * float64(statics[i].distance)
	}
	return int64(sum)
}

func (p *MetricComponent) TimeDistanceExpect() int64 {
	if p.sampleData.index == 0 {
		return 0
	}
	p.sampleData.mutex.Lock()
	defer p.sampleData.mutex.Unlock()
	return p.timeDistanceExpect()
}

func (p *MetricComponent) TimeDistanceStandard() (int64, int64, int64, int64) {
	if p.sampleData.index == 0 {
		return 0, 0, 0, 0
	}
	p.sampleData.mutex.Lock()
	defer p.sampleData.mutex.Unlock()
	expect, disVar, average := p.timeDistanceVar()
	return expect, int64(disVar), int64(math.Sqrt(float64(disVar))), average
}

func (p *MetricComponent) timeDistanceVar() (int64, int64, int64) {
	if p.sampleData.index == 0 {
		return 0, 0, 0
	}
	var disVar float64
	expect := p.timeDistanceExpect()
	average := p.timeDistanceAverage()
	for i := 0; i < p.sampleData.index; i++ {
		disVar += math.Pow(math.Abs(float64(expect-p.sampleData.timeMetric[i].distance)), 2)
	}

	return expect, int64(disVar / float64(p.sampleData.index)), average
}

func (p *MetricComponent) TimeDistanceVar() (int64, int64, int64) {
	if p.sampleData.index == 0 {
		return 0, 0, 0
	}
	p.sampleData.mutex.Lock()
	defer p.sampleData.mutex.Unlock()
	expect, disVar, average := p.timeDistanceVar()
	return expect, disVar, average
}

func (p *MetricComponent) timeDistanceAverage() int64 {
	if p.sampleData.index == 0 {
		return 0
	}
	var sumDistance int64
	for i := 0; i < p.sampleData.index; i++ {
		metricElement := p.sampleData.timeMetric[i]
		if metricElement.endTime == 0 {
			sumDistance += time.Now().UnixNano()/int64(time.Microsecond) - metricElement.startTime
		} else {
			sumDistance += metricElement.endTime - metricElement.startTime
		}

	}

	return sumDistance / int64(p.sampleData.index)
}

func (p *MetricComponent) TimeDistanceAverage() int64 {
	p.sampleData.mutex.Lock()
	defer p.sampleData.mutex.Unlock()
	return p.timeDistanceAverage()
}

func (p *MetricComponent) sendMetricRequest(client *network.PeerClient) {
	nanoTime := time.Now().UnixNano()
	p.sampleData.mutex.Lock()
	if p.sampleData.index >= p.sampleSize && p.sampleData.timeMetric[p.sampleSize-1].startTime > 0 {
		for i := 0; i < p.sampleSize-1; i++ {
			p.sampleData.timeMetric[i] = p.sampleData.timeMetric[i+1]
		}
		p.sampleData.timeMetric[p.sampleSize-1] = TimeDistance{startTime: nanoTime / int64(time.Microsecond), endTime: 0}
	} else {
		p.sampleData.timeMetric[p.sampleData.index] = TimeDistance{startTime: nanoTime / int64(time.Microsecond), endTime: 0}
	}
	if p.sampleData.index+1 >= p.sampleSize {
		p.sampleData.isFull = true
		p.sampleData.index = p.sampleSize - 1
	}
	workingIndex := p.sampleData.index
	p.sampleData.index++
	p.sampleData.mutex.Unlock()

	resp, err := client.Request(context.Background(), &protobuf.MetricRequest{SendTimestamp: nanoTime, RawData: p.randRaw})
	if err != nil {
		log.Errorf("send metric request err:%s, client.addr:%s", err.Error(), client.Address)
		return
	}
	if resp.(*protobuf.MetricResponse).SendTimestamp != nanoTime {
		log.Errorf("receive response message is not match its sent request message, resp.Timestamp:%d, request.Timestamp:%d",
			resp.(*protobuf.MetricResponse).SendTimestamp, nanoTime)
	}
	//NOTICE: request is sync operation, so need to use a workingIndex to backup current index value avoid being covered
	p.sampleData.timeMetric[workingIndex].endTime = time.Now().UnixNano() / int64(time.Microsecond)
}

func (p *MetricComponent) echoMetricValues(client *network.PeerClient) {
	if p.sampleData.index == 0 {
		return
	}
	if p.sampleData.index+1 >= p.sampleSize && p.sampleData.timeMetric[p.sampleSize-1].startTime > 0 {
		if p.sampleData.timeMetric[p.sampleSize-1].endTime == 0 {
			p.sampleData.timeMetric[p.sampleSize-1].distance = time.Now().UnixNano()/int64(time.Microsecond) - p.sampleData.timeMetric[p.sampleSize-1].startTime
		} else {
			p.sampleData.timeMetric[p.sampleSize-1].distance = p.sampleData.timeMetric[p.sampleSize-1].endTime - p.sampleData.timeMetric[p.sampleSize-1].startTime
		}
		log.Infof("in MetricComponent, client addr:%s,dataSet is full:%d, current index:%d, latest once distance:%dus, sample time interval:%dms, sample package size:%d bytes, sample size:%d",
			client.ID.Address, p.sampleData.isFull, p.sampleData.index-1, p.sampleData.timeMetric[p.sampleSize-1].distance, p.sampleInterval/time.Millisecond, p.packageSize, p.sampleSize)
	} else {
		if p.sampleData.timeMetric[p.sampleData.index-1].endTime == 0 {
			p.sampleData.timeMetric[p.sampleData.index-1].distance = time.Now().UnixNano()/int64(time.Microsecond) - p.sampleData.timeMetric[p.sampleData.index-1].startTime
		} else {
			p.sampleData.timeMetric[p.sampleData.index-1].distance = p.sampleData.timeMetric[p.sampleData.index-1].endTime - p.sampleData.timeMetric[p.sampleData.index-1].startTime
		}
		log.Infof("in MetricComponent, client addr:%s, dataSet is full:%d, current index:%d, latest once distance:%dus, sample time interval:%dms, sample package size:%d bytes, sample size:%d",
			client.ID.Address, p.sampleData.isFull, p.sampleData.index-1, p.sampleData.timeMetric[p.sampleData.index-1].distance, p.sampleInterval/time.Millisecond, p.packageSize, p.sampleSize)
	}
	expect, disVar, standard, average := p.TimeDistanceStandard()
	log.Infof("in MetricComponent, client addr:%s, expect value:%dus, disVar value:%dus, standard value:%dus, average value:%dus",
		client.ID.Address, expect, disVar, standard, average)
}

func (p *MetricComponent) PeerConnect(client *network.PeerClient) {
	go func(client *network.PeerClient) {
		t := time.NewTicker(p.sampleInterval)
		for {
			select {
			case <-t.C:
				if client == nil {
					return
				}
				p.echoMetricValues(client)
				go p.sendMetricRequest(client)
			case <-p.net.Kill:
				return
			case <-client.CloseSignal:
				return
			}
		}
	}(client)
}

func (p *MetricComponent) Receive(ctx *network.ComponentContext) error {
	switch ctx.Message().(type) {
	case *protobuf.MetricRequest:
		ctx.Reply(context.Background(), &protobuf.MetricResponse{SendTimestamp: ctx.Message().(*protobuf.MetricRequest).SendTimestamp, RawData: ctx.Message().(*protobuf.MetricRequest).RawData})
	case *protobuf.MetricResponse:
	}
	return nil
}

func (p *MetricComponent) PeerDisconnect(client *network.PeerClient) {
	m, ok := client.Network.NetDistanceMetric.Load(client.Address)
	if ok == false {
		log.Errorf("load distance metric false in peer disconnect, addr:%s, does not exists.", client.Address)
		return
	}
	log.Warnf("client addr:%s, start unix time:%d, package count:%d, time spand count for all package: %d", client.Address,
		m.(network.NetDistanceMetric).StartTime, m.(network.NetDistanceMetric).PackageCounter, m.(network.NetDistanceMetric).TimeCounter)
}
