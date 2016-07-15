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
		fmt.Fprintf(os.Stderr, "  %s -path /key -children 127.0.0.1\n", os.Args[0])
	}

	path := ""
	children := false

	flag.StringVar(&path, "path", "/", "zookeeper node path")
	flag.BoolVar(&children, "children", false, "monitor child nodes")
	flag.Parse()

	conn, _, err := zk.Connect(flag.Args(), time.Second)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()

	if children {
		for {
			children, _, ch, err := conn.ChildrenW(path)
			if err != nil {
				fmt.Println(err)
				return
			}

			fmt.Printf("children:\n")

			for _, c := range children {
				fmt.Printf("  %s\n", c)
			}

			e := <-ch
			fmt.Printf("event: %s\n", e.Type.String())
		}
	} else {
		for {
			data, _, ch, err := conn.GetW(path)
			if err != nil {
				fmt.Println(err)
				return
			}

			fmt.Printf("value: %s\n", string(data))

			e := <-ch
			fmt.Printf("event: %s\n", e.Type.String())
		}
	}
}
