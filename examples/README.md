### Example session

Build go-zookeeper examples:

```
$ go get github.com/samuel/go-zookeeper/...
```

Start testing cluster:

```
$ zk_cluster
...
Started servers:

  127.0.0.1:12845
  127.0.0.1:12848
  127.0.0.1:12851
```

Create value:

```
$ zk_write -path /hello -data foo 127.0.0.1:12845
```

Read the value:

```
$ zk_read -path /hello 127.0.0.1:12845 2>/dev/null
foo
```

Watch value changes:

```
$ zk_watch -path /hello 127.0.0.1:12845 2>/dev/null
value: foo
```

Change value from another terminal:

```
$ zk_write -path /hello -data bar 127.0.0.1:12845
```

Watch output will be:

```
event: EventNodeDataChanged
value: bar
```

Watch value children:

```
$ zk_watch -path /hello -children 127.0.0.1:12845 2>/dev/null
children:
```

Create children from another terminal:

```
$ zk_write -path /hello/1 127.0.0.1:12845
$ zk_write -path /hello/2 127.0.0.1:12845
```

Watch output will be:

```
event: EventNodeChildrenChanged
children:
  1
event: EventNodeChildrenChanged
children:
  1
  2
```

Print tree:

```
$ zk_tree -path /hello 127.0.0.1:12845 2>/dev/null
-- /hello
-- /hello/1
-- /hello/2
```
