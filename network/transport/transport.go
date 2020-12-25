package transport

import "time"

// Layer represents a transport protocol layer.
type Layer interface {
	Listen(address string) (interface{}, error)
	Dial(address string, timeout time.Duration) (interface{}, error)
	TLSListen(string, string, string, string) (interface{}, error)
	TLSDial(string, time.Duration, string, string, string) (interface{}, error)
}
