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
		fmt.Fprintf(os.Stderr, "  %s -path /key 127.0.0.1\n", os.Args[0])
	}

	path := ""

	flag.StringVar(&path, "path", "/", "zookeeper node path")
	flag.Parse()

	conn, _, err := zk.Connect(flag.Args(), time.Second)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()

	data, _, err := conn.Get(path)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("%s\n", string(data))
}
