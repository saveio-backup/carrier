/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-07-10
*/
package common

import "fmt"

type ProxiedAddr struct {
	Net  string
	Host string
	Port int
}

func (a *ProxiedAddr) Network() string {
	return a.Net
}

func (a *ProxiedAddr) String() string {
	return fmt.Sprintf("%s:%d", a.Host, a.Port)
}
