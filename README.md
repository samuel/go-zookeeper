go-zookeeper
============

Native ZooKeeper client for Go, forked from https://github.com/samuel/go-zookeeper

Added `set` and `create`, `create` still errors out
Left in commented debug statements


`create` errors is as follows:

```
  path: /test/set2
  data: this is a test
calling create
2012/12/12 11:53:28 ---> in Create
2012/12/12 11:53:28 this is a test
2012/12/12 11:53:28 ----------create request------------------------
2012/12/12 11:53:28 &{xid:1 pkt:0xf840085040 recvStruct:0xf8400398d0 recvChan:0xf840089050}
2012/12/12 11:53:28 &{requestHeader:{Xid:1 Opcode:1} Path:/test/set2 Data:[116 104 105 115 32 105 115 32 97 32 116 101 115 116] Acl:[{Perms:31 Id:{Scheme:world Id:0}}] Flags:0}
2012/12/12 12:01:34 --------- +||||+ -----> req =>&{xid:1 pkt:0xf840097040 recvStruct:0xf8400398d0 recvChan:0xf84009b050}
2012/12/12 12:01:34 --------- +||||+ -----> req.pkt =>&{requestHeader:{Xid:1 Opcode:1} Path:/test/set2 Data:[116 104 105 115 32 105 115 32 97 32 116 101 115 116] Acl:[{Perms:31 Id:{Scheme:world Id:0}}] Flags:0}
2012/12/12 12:01:34 --------- +||||+ -----> n =>65
2012/12/12 12:01:34 --------- +||||+ -----> buf =>[0 0 0 65 0 0 0 1 0 0 0 1 0 0 0 10 47 116 101 115 116 47 115 101 116 50 0 0 0 14 116 104 105 115 32 105 115 32 97 32 116 101 115 116 0 0 0 17 0 0 0 31 0 0 0 5 119 111 114 108 100 0 0 0 0 0 0 0 0 0 0 0 0 ..clipped.. 0 0 0 0 0]
2012/12/12 12:01:34 ----------create response------------------------
2012/12/12 12:01:34 &{responseHeader:{Xid:1 Zxid:21475566672 Err:-5} Path:2?mnЏe??????*?}
2012/12/12 12:01:34 err:<nil>
2012/12/12 12:01:34 rpath:2?mnЏe??????*?
2012/12/12 12:01:34 <--- out Create
create completed
done
```
