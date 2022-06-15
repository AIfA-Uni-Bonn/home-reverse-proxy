package doproxy

import (
	"log"
	"net/http"
)

// documentations
// https://www.alexedwards.net/blog/serving-static-sites-with-go

type proxy_service struct {
	name string
}

func Handle_proxy_request(w http.ResponseWriter, r *http.Request) {
	log.Printf("url-request: %v", r.URL)
	//proxy.ServeHTTP(w, r)
	http.NotFound(w, r)
}
