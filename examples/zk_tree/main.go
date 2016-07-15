package main

import (
	"flag"
	"fmt"
	"github.com/samuel/go-zookeeper/zk"
	"os"
	"path"
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
	maxdepth := -1

	flag.StringVar(&path, "path", "/", "zookeeper node path")
	flag.IntVar(&maxdepth, "depth", -1, "maximum depth")
	flag.Parse()

	conn, _, err := zk.Connect(flag.Args(), time.Second)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()

	traverse(conn, path, 0, maxdepth)
}

func traverse(conn *zk.Conn, root string, depth, maxdepth int) {
	if depth >= maxdepth && maxdepth >= 0 {
		return
	}

	fmt.Printf("-- %s\n", root)

	children, _, err := conn.Children(root)
	if err != nil {
		fmt.Println(err)
		return
	}

	for _, c := range children {
		traverse(conn, path.Join(root, c), depth+1, maxdepth)
	}
}
