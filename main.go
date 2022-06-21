package main

// written by: Oliver Cordes 2022-06-17
// changed by: Oliver Cordes 2022-06-19

import (
	"aifa-uni-bonn/home-reverse-proxy/doproxy"
	"fmt"
	"log"
	"net/http"
)

func main() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	doproxy.Init_doproxy()
	// initialize a reverse proxy and pass the actual backend server url here
	//proxy, err := NewProxy("http://web-www2019.astro.uni-bonn.de")
	//if err != nil {
	//	panic(err)
	//}

	// handle all requests to your server using the proxy
	//http.HandleFunc("/", ProxyRequestHandler(proxy))
	http.HandleFunc("/", doproxy.Handle_proxy_request)

	//log.Fatal(http.ListenAndServe(":8080", nil))
	log.Printf("Starting server on port: %v\n", doproxy.Server_port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%v", doproxy.Server_port), nil))
}
