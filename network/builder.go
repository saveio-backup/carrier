package network

import (
	"reflect"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/saveio/carrier/crypto"
	"github.com/saveio/carrier/crypto/blake2b"
	"github.com/saveio/carrier/crypto/ed25519"
	"github.com/saveio/carrier/metric/carrier-metrics"
	"github.com/saveio/carrier/network/transport"
	"github.com/saveio/carrier/peer"
	"github.com/saveio/themis/common/log"
)

const (
	defaultAddress = "tcp://localhost:8588"
)

var (
	// ErrStrDuplicateComponent returns if the Component has already been registered
	// with the builder
	ErrStrDuplicateComponent = "builder: Component %s is already registered"
	// ErrStrNoAddress returns if no address was given to the builder
	ErrStrNoAddress = "builder: network requires public server IP for peers to connect to"
	// ErrStrNoKeyPair returns if no keypair was given to the builder
	ErrStrNoKeyPair = "builder: cryptography keys not provided to Network; cannot create node ID"
)

// Builder is a Address->processors struct
type Builder struct {
	opts options

	keys           *crypto.KeyPair
	address        string
	listenAddr     string
	Components     *ComponentList
	ComponentCount int

	transports *sync.Map
}

var defaultBuilderOptions = options{
	connectionTimeout:    defaultConnectionTimeout,
	signaturePolicy:      ed25519.New(),
	hashPolicy:           blake2b.New(),
	recvWindowSize:       defaultReceiveWindowSize,
	sendWindowSize:       defaultSendWindowSize,
	recvBufferSize:       defaultRecvBufferSize,
	writeBufferSize:      defaultWriteBufferSize,
	writeFlushLatency:    defaultWriteFlushLatency,
	perBlockWriteTimeout: defaultWriteTimeout,
}

// A BuilderOption sets options such as connection timeout and cryptographic // policies for the network
type BuilderOption func(*options)

// ConnectionTimeout returns a NetworkOption that sets the timeout for
// establishing new connections (default: 60 seconds).
func ConnectionTimeout(d time.Duration) BuilderOption {
	return func(o *options) {
		o.connectionTimeout = d
	}
}

// SignaturePolicy returns a BuilderOption that sets the signature policy
// for the network (default: ed25519).
func SignaturePolicy(policy crypto.SignaturePolicy) BuilderOption {
	return func(o *options) {
		o.signaturePolicy = policy
	}
}

// HashPolicy returns a BuilderOption that sets the hash policy for the network
// (default: blake2b).
func HashPolicy(policy crypto.HashPolicy) BuilderOption {
	return func(o *options) {
		o.hashPolicy = policy
	}
}

// RecvWindowSize returns a BuilderOption that sets the receive buffer window
// size (default: 4096).
func RecvWindowSize(recvWindowSize int) BuilderOption {
	return func(o *options) {
		o.recvWindowSize = recvWindowSize
	}
}

// SendWindowSize returns a BuilderOption that sets the send buffer window
// size (default: 4096).
func SendWindowSize(sendWindowSize int) BuilderOption {
	return func(o *options) {
		o.sendWindowSize = sendWindowSize
	}
}

// WriteBufferSize returns a BuilderOption that sets the write buffer size
// (default: 4096 bytes).
func WriteBufferSize(byteSize int) BuilderOption {
	return func(o *options) {
		o.writeBufferSize = byteSize
	}
}

func ReceiveBufferSize(byteSize int) BuilderOption {
	return func(o *options) {
		o.recvBufferSize = byteSize
	}
}

// WriteFlushLatency returns a BuilderOption that sets the write flush interval
// (default: 50ms).
func WriteFlushLatency(d time.Duration) BuilderOption {
	return func(o *options) {
		o.writeFlushLatency = d
	}
}

// WriteTimeout returns a BuilderOption that sets the write timeout
// (default: 4096).
func WriteTimeout(d int) BuilderOption {
	return func(o *options) {
		o.perBlockWriteTimeout = d
	}
}

// NewBuilder returns a new builder with default options.
func NewBuilder() *Builder {
	builder := &Builder{
		opts:       defaultBuilderOptions,
		address:    defaultAddress,
		keys:       ed25519.RandomKeyPair(),
		transports: new(sync.Map),
	}

	// Register default transport layers.
	builder.RegisterTransportLayer("tcp", transport.NewTCP())
	builder.RegisterTransportLayer("kcp", transport.NewKCP())
	builder.RegisterTransportLayer("udp", transport.NewUDP())
	builder.RegisterTransportLayer("quic", transport.NewQuic())

	return builder
}

// NewBuilderWithOptions returns a new builder with specified options.
func NewBuilderWithOptions(opt ...BuilderOption) *Builder {
	builder := NewBuilder()

	for _, o := range opt {
		o(&builder.opts)
	}

	return builder
}

// SetKeys pair created from crypto.KeyPair.
func (builder *Builder) SetKeys(pair *crypto.KeyPair) {
	builder.keys = pair
}

// SetAddress sets the host address for the network.
func (builder *Builder) SetAddress(address string) {
	builder.address = address
}

func (builder *Builder) SetListenAddr(address string) {
	builder.listenAddr = address
}

// AddComponentWithPriority registers a new Component onto the network with a set priority.
func (builder *Builder) AddComponentWithPriority(priority int, Component ComponentInterface) error {
	// Initialize Component list if not exist.
	if builder.Components == nil {
		builder.Components = NewComponentList()
	}

	if !builder.Components.Put(priority, Component) {
		return errors.Errorf(ErrStrDuplicateComponent, reflect.TypeOf(Component).String())
	}

	return nil
}

func (builder *Builder) DeleteComponent(Component ComponentInterface) bool {
	return builder.Components.Delete(Component)
}

// AddComponent register a new Component onto the network.
func (builder *Builder) AddComponent(Component ComponentInterface) error {
	err := builder.AddComponentWithPriority(builder.ComponentCount, Component)
	if err == nil {
		builder.ComponentCount++
	}
	return err
}

// RegisterTransportLayer registers a transport layer to the network keyed by its name.
//
// Example: builder.RegisterTransportLayer("kcp", transport.NewKCP())
func (builder *Builder) RegisterTransportLayer(name string, layer transport.Layer) {
	builder.transports.Store(name, layer)
}

// ClearTransportLayers removes all registered transport layers from the builder.
func (builder *Builder) ClearTransportLayers() {
	builder.transports = new(sync.Map)
}

// Build verifies all parameters of the network and returns either an error due to
// misconfiguration, or a *Network.
func (builder *Builder) Build() (*Network, error) {
	if builder.keys == nil {
		return nil, errors.New(ErrStrNoKeyPair)
	}

	if len(builder.address) == 0 {
		return nil, errors.New(ErrStrNoAddress)
	}

	// Initialize Component list if not exist.
	if builder.Components == nil {
		builder.Components = NewComponentList()
	} else {
		builder.Components.SortByPriority()
	}

	unifiedAddress, err := ToUnifiedAddress(builder.address)
	if err != nil {
		return nil, err
	}

	id := peer.CreateID(unifiedAddress, builder.keys.PublicKey)

	unifiedListenAddr, err := ToUnifiedAddress(builder.listenAddr)
	if err != nil {
		return nil, err
	}
	net := &Network{
		opts:                        builder.opts,
		ID:                          id,
		keys:                        builder.keys,
		Address:                     unifiedAddress,
		ListenAddr:                  unifiedListenAddr,
		Components:                  builder.Components,
		transports:                  builder.transports,
		listeningCh:                 make(chan struct{}),
		ProxyService:                Proxy{Finish: new(sync.Map), ConnectionEvent: make([]chan *ProxyEvent, defaultProxyNotifySize), WorkID: 0},
		Kill:                        make(chan struct{}),
		compressEnable:              true,
		compressAlgo:                GZIP,
		CompressCondition:           CompressCondition{Size: defaultCompressFileSize},
		createClientMutex:           new(sync.Mutex),
		metric:                      initMetric(),
		NetDistanceMetric:           new(sync.Map),
		DisableDispatchMsgGoroutine: false,
		Reporter:                    metrics.NewBandwidthCounter(),
		streamQueueLen:              defaultStreamQueueLen,
		bootstrapWaitSecond:         defaultBootstrapWaitSecond,
	}

	net.ConnMgr.peers = new(sync.Map)
	net.ConnMgr.connections = new(sync.Map)
	net.ConnMgr.connStates = new(sync.Map)
	net.ConnMgr.streams = new(sync.Map)

	net.transports.Range(func(protocol, _ interface{}) bool {
		net.ProxyService.Finish.Store(protocol, make(chan struct{}))
		return true
	})
	log.Info("carrier version:", Version)
	net.Init()

	return net, nil
}
