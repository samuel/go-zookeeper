package main

import (
	"fmt"
	"github.com/samuel/go-zookeeper/zk"
	"os"
)

func main() {
	cluster, err := zk.StartTestCluster(3, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer cluster.Stop()

	fmt.Printf("\nStarted servers:\n\n")

	for _, server := range cluster.Servers {
		fmt.Printf("  127.0.0.1:%d\n", server.Port)
	}

	fmt.Printf("\n")

	select {}
}
