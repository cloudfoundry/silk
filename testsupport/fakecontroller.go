package testsupport

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sync"

	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"

	. "github.com/onsi/gomega"
)

type FakeController struct {
	ifrit.Process
	handlerLock sync.Mutex
	handlers    map[string]*FakeHandler
}

type FakeHandler struct {
	LastRequestBody []byte
	ResponseCode    int
	ResponseBody    interface{}
}

func (f *FakeController) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	f.handlerLock.Lock()
	defer f.handlerLock.Unlock()
	var fakeHandler *FakeHandler
	for route, h := range f.handlers {
		if r.URL.Path == route {
			fakeHandler = h
		}
	}
	if fakeHandler == nil {
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte(fmt.Sprintf(`{}`)))
		return
	}

	fakeHandler.LastRequestBody = bodyBytes
	responseBytes, _ := json.Marshal(fakeHandler.ResponseBody)
	w.WriteHeader(fakeHandler.ResponseCode)
	w.Write(responseBytes)
}

func (f *FakeController) SetHandler(route string, handler *FakeHandler) {
	f.handlerLock.Lock()
	defer f.handlerLock.Unlock()
	f.handlers[route] = handler
}

func StartServer(serverListenAddr string, tlsConfig *tls.Config) *FakeController {
	fakeServer := &FakeController{
		handlers: make(map[string]*FakeHandler),
	}

	someServer := http_server.NewTLSServer(serverListenAddr, fakeServer, tlsConfig)

	members := grouper.Members{{
		Name:   "http_server",
		Runner: someServer,
	}}
	group := grouper.NewOrdered(os.Interrupt, members)
	monitor := ifrit.Invoke(sigmon.New(group))

	Eventually(monitor.Ready()).Should(BeClosed())
	fakeServer.Process = monitor
	return fakeServer
}

func (f *FakeController) Stop() {
	if f == nil {
		return
	}
	f.Process.Signal(os.Interrupt)
	Eventually(f.Process.Wait()).Should(Receive())
}
