package transport

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"strings"
	"time"

	"github.com/tjfoc/gmsm/sm2"
	"github.com/tjfoc/gmtls"
)

// TCP represents the TCP transport protocol alongside its respective configurable options.
type TCP struct {
	WriteBufferSize int
	ReadBufferSize  int
	NoDelay         bool
}

// NewTCP instantiates a new instance of the TCP transport protocol.
func NewTCP() *TCP {
	return &TCP{
		WriteBufferSize: 10000,
		ReadBufferSize:  10000,
		NoDelay:         false,
	}
}

func resolveTcpAddr(address string) string {
	if strings.HasPrefix(address, "tcp://") {
		return address[len("tcp://"):]
	}
	return address
}

func (t *TCP) Listen(address string) (interface{}, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	return interface{}(listener), nil
}

// Listen listens for incoming TCP connections on a specified port.
func (t *TCP) TLSListen(address string, certPath string, keyPath string, caPath string) (interface{}, error) {
	cert, err := gmtls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}

	caData, err := ioutil.ReadFile(caPath)
	if err != nil {
		return nil, err
	}
	pool := sm2.NewCertPool()
	ret := pool.AppendCertsFromPEM(caData)
	if !ret {
		return nil, errors.New("in tls listen, append certs from pem error")
	}

	tlsConfig := &gmtls.Config{
		Certificates: []gmtls.Certificate{cert},
		RootCAs:      pool,
		ClientAuth:   gmtls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
	}

	listener, err := gmtls.Listen("tcp", address, tlsConfig)
	if err != nil {
		return nil, err
	}
	return interface{}(listener), nil
}

func (t *TCP) Dial(address string, timeout time.Duration) (interface{}, error) {
	conn, err := net.DialTimeout("tcp", resolveTcpAddr(address), timeout)
	if err != nil {
		return nil, err
	}
	return interface{}(conn), nil
}

// Dial dials an address via. the TCP protocol.
func (t *TCP) TLSDial(address string, timeout time.Duration, caPath string, certPath string, keyPath string) (interface{}, error) {

	clientCertPool := sm2.NewCertPool()

	cacert, err := ioutil.ReadFile(caPath)
	if err != nil {
		fmt.Println("[p2p]load CA file fail", err)
		return nil, err
	}
	cert, err := gmtls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		fmt.Println("load x509 err:", err.Error())
		return nil, err
	}

	ret := clientCertPool.AppendCertsFromPEM(cacert)
	if !ret {
		return nil, errors.New("[p2p]failed to parse root certificate")
	}

	conf := &gmtls.Config{
		RootCAs:            clientCertPool,
		Certificates:       []gmtls.Certificate{cert},
		InsecureSkipVerify: true,
	}

	var dialer net.Dialer
	dialer.Timeout = timeout
	conn, err := gmtls.DialWithDialer(&dialer, "tcp", address, conf)
	if err != nil {
		return nil, err
	}
	return interface{}(conn), nil
}
