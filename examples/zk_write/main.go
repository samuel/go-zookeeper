package main

import (
	"flag"
	"fmt"
	"github.com/samuel/go-zookeeper/zk"
	"os"
	"time"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s -path /key -data value 127.0.0.1\n", os.Args[0])
	}

	path := ""
	data := ""

	flag.StringVar(&path, "path", "", "zookeeper node path")
	flag.StringVar(&data, "data", "", "zookeeper node data")
	flag.Parse()

	fmt.Printf("--- connecting to %v\n", flag.Args())

	conn, _, err := zk.Connect(flag.Args(), time.Second)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()

	exists, _, err := conn.Exists(path)
	if err != nil {
		fmt.Println(err)
		return
	}

	if !exists {
		fmt.Printf("--- creating %s\n", path)

		_, err := conn.Create(path, []byte(data), int32(0), zk.WorldACL(zk.PermAll))
		if err != nil {
			fmt.Println(err)
			return
		}
	} else {
		fmt.Printf("--- reading %s\n", path)

		_, s, err := conn.Get(path)
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Printf("--- version %d\n", s.Version)

		fmt.Printf("--- writing %s\n", path)

		_, err = conn.Set(path, []byte(data), s.Version)
		if err != nil {
			fmt.Println(err)
			return
		}
	}
}
