package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"code.cloudfoundry.org/debugserver"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/controller/config"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
)

func main() {
	if err := mainWithError(); err != nil {
		log.Fatalf("silk-controller error: %s", err)
	}
}

type BasicServer struct{}

func (*BasicServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func mainWithError() error {
	logger := lager.NewLogger("silk-controller")
	reconfigurableSink := lager.NewReconfigurableSink(
		lager.NewWriterSink(os.Stdout, lager.DEBUG),
		lager.INFO)
	logger.RegisterSink(reconfigurableSink)
	logger.Info("starting")

	var configFilePath string
	flag.StringVar(&configFilePath, "config-file", "", "path to config file")
	flag.Parse()

	conf, err := config.ReadFromFile(configFilePath)
	if err != nil {
		return fmt.Errorf("loading config: %s", err)
	}

	debugServerAddress := fmt.Sprintf("127.0.0.1:%d", conf.DebugServerPort)
	mainServerAddress := fmt.Sprintf("%s:%d", conf.ListenHost, conf.ListenPort)
	httpServer := http_server.New(mainServerAddress, &BasicServer{})
	members := grouper.Members{
		{"http_server", httpServer},
		{"debug-server", debugserver.Runner(debugServerAddress, reconfigurableSink)},
	}

	group := grouper.NewOrdered(os.Interrupt, members)
	monitor := ifrit.Invoke(sigmon.New(group))

	err = <-monitor.Wait()
	if err != nil {
		return fmt.Errorf("wait returned error: %s", err)
	}

	logger.Info("exited")
	return nil
}
