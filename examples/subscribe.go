/*
    An example of how to subscribe to receive continual updates for running
    processes.
*/

package main


import (
    "os"
    "os/signal"
    "syscall"
    "fmt"
    "time"
    "strconv"
    "github.com/littleinc/go-zookeeper"
)


var ACLOpen = zk.WorldACL(zk.PermAll)


func worker(changes chan zk.WatchChange) {
    for change := range changes {
        fmt.Printf("STATE UPDATED: %+v\n", change)
    }
}


func main() {
    c, _, _ := zk.Connect([]string{"zk01.foo.msgme.im"}, time.Second)
    c.Create("/"+strconv.Itoa(os.Getpid()), []byte{}, zk.FlagEphemeral, ACLOpen)
    go worker(c.ChildrenWSubscribe("/"))

    exit := make(chan os.Signal, 1)
    signal.Notify(exit, syscall.SIGINT, syscall.SIGTERM)
    <-exit

    signal.Stop(exit)
    c.Close()
    time.Sleep(time.Millisecond)
    fmt.Println("bye.")
}
