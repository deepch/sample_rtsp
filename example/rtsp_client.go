package main

import (
	"fmt"
	"github.com/deepch/rtsp"
)

func main() {
	count := 0
	Client := RtspClientNew()
	Client.debug = false
	if err := Client.Open("rtsp://admin:123456@171.25.235.18/mpeg4"); err != nil {
		fmt.Println("[RTSP] Error", err)
	} else {
		for {
			select {
			case <-Client.signals:
				fmt.Println("Exit signals by rtsp")
				return
			case data := <-Client.outgoing:
				count += len(data)
				fmt.Println("recive  rtp packet size", len(data), "recive all packet size", count)
			}
		}
	}
	Client.Close()
}
