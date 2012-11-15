package zk

const (
	protocolVersion = 0

	defaultPort = 2181
)

const (
	opNotify       = 0
	opCreate       = 1
	opDelete       = 2
	opExists       = 3
	opGetData      = 4
	opSetData      = 5
	opGetAcl       = 6
	opSetAcl       = 7
	opGetChildren  = 8
	opSync         = 9
	opPing         = 11
	opGetChildren2 = 12
	opCheck        = 13
	opMulti        = 14
	opClose        = -11
	opSetAuth      = 100
	opSetWatches   = 101
	// Not in protocol, used internally
	opWatcherEvent = -2
)

const (
	EventNodeCreated         = EventType(1)
	EventNodeDeleted         = EventType(2)
	EventNodeDataChanged     = EventType(3)
	EventNodeChildrenChanged = EventType(4)

	EventSession     = EventType(-1)
	EventNotWatching = EventType(-2)
)

var (
	eventNames = map[EventType]string{
		EventNodeCreated:         "EventNodeCreated",
		EventNodeDeleted:         "EventNodeDeleted",
		EventNodeDataChanged:     "EventNodeDataChanged",
		EventNodeChildrenChanged: "EventNodeChildrenchanged",
		EventSession:             "EventSession",
		EventNotWatching:         "EventNotWatching",
	}
)

const (
	StateUnknown      = State(-1)
	StateDisconnected = State(0)
	// StateNoSyncConnected   = State(1) // deprecated, unused, never generated
	StateSyncConnected     = State(3)
	StateAuthFailed        = State(4)
	StateConnectedReadOnly = State(5)
	StateSaslAuthenticated = State(6)
	StateExpired           = State(-112)

	// stateAuthFailed = -113??

	StateConnected  = State(100)
	StateHasSession = State(101)
)

var (
	stateNames = map[State]string{
		StateUnknown:           "StateUnknown",
		StateDisconnected:      "StateDisconnected",
		StateSyncConnected:     "StateSyncConnected",
		StateAuthFailed:        "StateAuthFailed",
		StateConnectedReadOnly: "StateConnectedReadOnly",
		StateSaslAuthenticated: "StateSaslAuthenticated",
		StateExpired:           "StateExpired",
		StateConnected:         "StateConnected",
		StateHasSession:        "StateHasSession",
	}
)

type State int32

func (s State) String() string {
	if name := stateNames[s]; name != "" {
		return name
	}
	return "Unknown"
}

const (
	errOk = 0
	// System and server-side errors
	errSystemError          = -1
	errRuntimeInconsistency = -2
	errDataInconsistency    = -3
	errConncetionLoss       = -4
	errMarshallingError     = -5
	errUnimplemented        = -6
	errOperationTimeout     = -7
	errBadArguments         = -8
	errInvalidState         = -9
	// API errors
	errApiError                = -100
	errNoNode                  = -101 // *
	errNoAuth                  = -102
	errBadVersion              = -103 // *
	errNoChildrenForEphemerals = -108
	errNodeExists              = -110 // *
	errNotEmpty                = -111
	errSessionExpired          = -112
	errInvalidCallback         = -113
	errInvalidAcl              = -114
	errAuthFailed              = -115
	errClosing                 = -116
	errNothing                 = -117
	errSessionMoved            = -118
)

const (
	PermRead = 1 << iota
	PermWrite
	PermCreate
	PermDelete
	PermAdmin
	PermAll = 0x1f
)

var (
	emptyPassword = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	opNames       = map[int32]string{
		opNotify:       "notify",
		opCreate:       "create",
		opDelete:       "delete",
		opExists:       "exists",
		opGetData:      "getData",
		opSetData:      "setData",
		opGetAcl:       "getACL",
		opSetAcl:       "setACL",
		opGetChildren:  "getChildren",
		opSync:         "sync",
		opPing:         "ping",
		opGetChildren2: "getChildren2",
		opCheck:        "check",
		opMulti:        "multi",
		opClose:        "close",
		opSetAuth:      "setAuth",
		opSetWatches:   "setWatches",

		opWatcherEvent: "watcherEvent",
	}
)

type EventType int32

func (t EventType) String() string {
	if name := eventNames[t]; name != "" {
		return name
	}
	return "Unknown"
}
