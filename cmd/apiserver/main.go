package main

import (
	"fantom-api-graphql/internal/config"

	"log"
	/* "net/http" */
	"os"
	"os/signal"
	"syscall"
)

// main initializes the API server and starts it when ready.
func main() {
	// get the configuration to prepare the server
	cfg, err := config.Load()
	if nil != err {
		log.Fatal(err)
	}

	// make the server instance
	api, err := NewApiServer(cfg)
	if err != nil {
		log.Fatal(err)
	}

	// run the server
	setupSignals(api)
	api.Run()
}

// setupSignals creates a system signal listener and handles graceful termination upon receiving one.
func setupSignals(api *ApiServer) {
	// make the signal consumer
	ts := make(chan os.Signal, 1)
	signal.Notify(ts, syscall.SIGINT, syscall.SIGTERM)

	// start monitoring
	go func() {
		// wait for the signal to stop
		<-ts
		api.Stop()

		// sign out
		os.Exit(0)
	}()
}
