package main

import (
	"aifa-uni-bonn/home-reverse-proxy/doproxy"
	"log"
	"net/http"
)

func main() {
	doproxy.Init_doproxy()
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	// initialize a reverse proxy and pass the actual backend server url here
	//proxy, err := NewProxy("http://web-www2019.astro.uni-bonn.de")
	//if err != nil {
	//	panic(err)
	//}

	// handle all requests to your server using the proxy
	//http.HandleFunc("/", ProxyRequestHandler(proxy))
	http.HandleFunc("/", doproxy.Handle_proxy_request)

	log.Fatal(http.ListenAndServe(":8080", nil))
}
