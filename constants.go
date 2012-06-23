package zk

const (
	protocolVersion = 0
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
	eventTypeNone                = -1
	eventTypeNodeCreated         = 1
	eventTypeNodeDeleted         = 2
	eventTypeNodeDataChanged     = 3
	eventTypeNodeChildrenChanged = 4
)

const (
	stateUnknown           = -1
	stateDisconnected      = 0
	stateNoSyncConnected   = 1 // deprecated, unused, never generated
	stateSyncConnected     = 3
	stateAuthFailed        = 4
	stateConnectedReadOnly = 5
	stateSaslAuthenticated = 6
	stateExpired           = -112

	// stateAuthFailed = -113??

	stateUnassociated = 99
)

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
