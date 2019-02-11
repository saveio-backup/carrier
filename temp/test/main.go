/**
 * Description:
 * Author: LiYong Zhang
 * Create: 2019-01-31 
*/
package main

import (
	"time"
	"fmt"
)

func main(){
	go test()
	select{}
}

func test(){
	var k <-chan time.Time
	//go func() {
	//	k=time.Tick(5 *time.Millisecond)
	//}()
	k=time.Tick(500*time.Millisecond)
	for   {
		select {
		case <-k:
			fmt.Println("keepalive")


		}

	}
}