package zk

type WatchChange struct {
    Ev       Event
    Children []string
}

type watchSub struct {
    conn    *Conn
    path    string
    updates chan WatchChange
}

func (conn *Conn) ChildrenWSubscribe(path string) chan WatchChange {
    s := &watchSub{
        conn:    conn,
        path:    path,
        updates: make(chan WatchChange),
    }
    go s.loop()
    return s.updates
}

func (s *watchSub) loop() {
    children, _, refresh, err := s.conn.ChildrenW(s.path)
    updates := s.updates
    ev := Event{EventNodeChildrenChanged, StateSyncConnected, s.path, nil}
    ev.Err = err
    update := WatchChange{ev, children}

    for {
        select{
            case <-refresh:
                children, _, refresh, err = s.conn.ChildrenW(s.path)
                if err == ErrConnectionClosed {
                    close(s.updates)
                    return
                }
                updates = s.updates
                ev.Err = err
                update = WatchChange{ev, children}

            case updates <- update:
                updates = nil
        }
    }
}
