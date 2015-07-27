package main

import (
	"fmt"
	"time"

	"../zk"
)

func main() {

	servers := []string{"192.168.28.191:2181"}
	_, _, err := ConnectWithTimeout(servers, 2*time.Second, 5*time.Second)
	if err != nil {
		fmt.Println("success")
	}
	/*
		c, _, err := zk.Connect([]string{"127.0.0.1"}, time.Second) //*10)
		if err != nil {
			panic(err)
		}
		children, stat, ch, err := c.ChildrenW("/")
		if err != nil {
			panic(err)
		}
		fmt.Printf("%+v %+v\n", children, stat)
		e := <-ch
		fmt.Printf("%+v\n", e)
	*/
}
