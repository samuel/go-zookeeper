package main

import (
	"fmt"
	"github.com/ngaut/go-zookeeper/zk"
	"time"
)

func main() {

	servers := []string{"192.168.28.191:2181"}

	conf := zk.ConnConf{
		RecvTimeout:    5 * time.Second,
		ConnTimeout:    5 * time.Second,
		SessionTimeout: 30000,
	}

	c, _, err := zk.ConnectWithConf(servers, conf)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("connect success")

		time.Sleep(20 * time.Second)

		data, stat, err := c.Get("/zk/codis")
		if err != nil {
			fmt.Printf("get error : %+v", err)
		} else {
			fmt.Printf("get success : data: %+v stat: %+v", data, stat)
		}
		_, stat, ch, err := c.ChildrenW("/zk")
		if err != nil {
			//fmt.Printf("%+v", err)
		}
		//fmt.Printf("%+v %+v\n", children, stat)
		e := <-ch
		fmt.Printf("%+v\n", e)
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
