/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-05-30
 */
package transport

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"strings"
	"time"

	"context"

	"github.com/lucas-clemente/quic-go"
)

const (
	defaultHandshakeTimeout = time.Millisecond * 100
	defaultIdleTimeout      = time.Second * 15
)

type Quic struct {
}

// NewQuic instantiates a new instance of the Quic protocol.
func NewQuic() *Quic {
	return &Quic{}
}

func resolveQuicAddr(address string) string {
	if strings.HasPrefix(address, "quic://") {
		return address[len("quic://"):]
	}
	return address
}

// Listen listens for incoming Quic connections on a specified port.
func (t *Quic) Listen(address string) (interface{}, error) {
	listener, err := quic.ListenAddr(address, generateTLSConfig(), &quic.Config{KeepAlive: true, IdleTimeout: defaultIdleTimeout, HandshakeTimeout: defaultHandshakeTimeout})
	if err != nil {
		return nil, err
	}

	return interface{}(listener), nil
}

func (t *Quic) Dial(address string, timeout time.Duration) (interface{}, error) {
	session, err := quic.DialAddr(resolveQuicAddr(address), &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"quic-proxy"}}, &quic.Config{KeepAlive: true, IdleTimeout: defaultIdleTimeout, HandshakeTimeout: defaultHandshakeTimeout})
	if err != nil {
		return nil, err
	}

	stream, err := session.OpenStreamSync(context.Background())
	if err != nil {
		return nil, err
	}

	return interface{}(stream), nil
}

// Setup a bare-bones TLS config for the server
func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{Certificates: []tls.Certificate{tlsCert}, NextProtos: []string{"quic-proxy"}}
}
