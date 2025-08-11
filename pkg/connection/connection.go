package connection

import (
	"github.com/xtaci/kcp-go/v5"
	"net"
	"net/http"
	"sync"
)

var MuClientListenerType sync.Mutex
var ClientListenerType = make(map[string]string)

var HttpServer = make(map[string]*http.Server)
var TCPServer = make(map[string]net.Listener)
var KCPServer = make(map[string]*kcp.Listener)
var StopChan = make(map[string]chan bool)
