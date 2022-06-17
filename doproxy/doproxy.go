package doproxy

import (
	"html/template"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// documentations
// https://www.alexedwards.net/blog/serving-static-sites-with-go

type proxy_service struct {
	name  string
	proxy *httputil.ReverseProxy
	start time.Time
}

var proxies map[string]proxy_service

func Init_doproxy() {
	proxies = make(map[string]proxy_service)
}

// NewProxy takes target host and creates a reverse proxy
func NewProxy(targetHost string) (*httputil.ReverseProxy, error) {
	url, err := url.Parse(targetHost)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(url)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		modifyRequest(req)
	}

	//proxy.ModifyResponse = modifyResponse()
	proxy.ErrorHandler = errorHandler()
	return proxy, nil
}

// handler for modifying requests
func modifyRequest(req *http.Request) {
	req.Header.Set("X-Proxy", "Simple-Reverse-Proxy")
}

// error handler for manipulating requests
func errorHandler() func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, req *http.Request, err error) {
		log.Printf("Got error while modifying response: %v \n", err)
		return
	}
}

// send_wait_page
//
// sends a waiting page for a specific user given by "s"
func send_wait_page(w http.ResponseWriter, s string) {
	// load the template
	tmpl, err := template.ParseFiles("templates/wait_for_docker.html")

	if err != nil {
		// Log the detailed error
		log.Print(err.Error())
		// Return a generic "Internal Server Error" message
		http.Error(w, http.StatusText(500), 500)
		return
	}

	// create the final web page
	err = tmpl.ExecuteTemplate(w, "layout", s)
	if err != nil {
		log.Print(err.Error())
	}
}

func create_proxy(s string) *httputil.ReverseProxy {
	url := "http://web-www2019.astro.uni-bonn.de"
	np, err := NewProxy(url)

	if err != nil {
		return nil
	}

	// create a new entry
	pe := proxy_service{name: s, proxy: np, start: time.Now()}

	proxies[s] = pe

	return np
}

// Handle_proxy_request
//
// works as the central station of the reverse proxy,
// checks, if a proxy is still running, or create a new proxy
// call the proxy if available, a creation may refer to a temp
// web page with an automatic reload!
func Handle_proxy_request(w http.ResponseWriter, r *http.Request) {
	log.Printf("url-request: %v - %v - %v", r.URL, r.RemoteAddr, r.Referer())
	//proxy.ServeHTTP(w, r)
	//http.NotFound(w, r)
	send_wait_page(w, "blubber")
}
