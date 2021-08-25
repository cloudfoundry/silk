package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
)

func main() {
	if len(os.Args) != 4 {
		log.Fatalf("expected 3 args\nport, statusCode, response")
	}
	port := os.Args[1]
	statusCode := os.Args[2]
	response := os.Args[3]
	code, err := strconv.Atoi(statusCode)
	if err != nil {
		log.Fatalf("status code must be an int")
	}

	server := http_server.New(
		fmt.Sprintf("127.0.0.1:%s", port),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(os.Stdout, "received request from %s\n", r.RemoteAddr)
			w.WriteHeader(code)
			w.Write([]byte(response))
		}),
	)

	members := grouper.Members{{"server", server}}
	group := grouper.NewOrdered(os.Interrupt, members)
	monitor := ifrit.Invoke(sigmon.New(group))

	err = <-monitor.Wait()
	if err != nil {
		log.Fatal(err)
	}
}
