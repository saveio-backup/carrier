package transport

import "time"

// Layer represents a transport protocol layer.
type Layer interface {
	Listen(port int) (interface{}, error)
	Dial(address string, timeout time.Duration) (interface{}, error)
}
