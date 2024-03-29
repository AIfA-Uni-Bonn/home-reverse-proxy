package main

// written by: Oliver Cordes 2022-06-17
// changed by: Oliver Cordes 2022-08-10

import (
	"aifa-uni-bonn/home-reverse-proxy/doproxy"
	"aifa-uni-bonn/home-reverse-proxy/pingpong"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-co-op/gocron"
)

var version string = "0.9.6.1"

func main() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	log.Printf("Running version: %s", version)
	doproxy.Init_doproxy()
	pingpong.Set_version(version)

	if doproxy.Culling {
		// setup the background culling service, if enabled
		s := gocron.NewScheduler(time.UTC)
		log.Printf("Setup a culling service every %v seconds...", doproxy.Culling_every)
		st := time.Now().Add(time.Second * time.Duration(doproxy.Culling_every))
		s.Every(doproxy.Culling_every).Seconds().StartAt(st).Do(doproxy.Service_culling)
		st = time.Now().Add(time.Second * 600)
		s.Every(3600).Seconds().StartAt(st).Do(doproxy.Service_deep_culling)

		// start the backgroud scheduler
		s.StartAsync()
	}

	// handle all requests to your server using the proxy
	http.HandleFunc("/", doproxy.Handle_proxy_request)

	// add pingpong for health checks
	http.HandleFunc("/ping", pingpong.Handle_ping_request)

	log.Printf("Starting server on port: %v\n", doproxy.Server_port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%v", doproxy.Server_port), nil))
}
