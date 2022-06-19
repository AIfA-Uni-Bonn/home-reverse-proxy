package doproxy

// written by: Oliver Cordes 2022-06-17
// changed by: Oliver Cordes 2022-06-19

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"time"

	"gopkg.in/yaml.v2"
)

// documentations
// https://www.alexedwards.net/blog/serving-static-sites-with-go

type proxy_service struct {
	name  string
	url   string
	proxy *httputil.ReverseProxy
	start time.Time
}

var proxies map[string]proxy_service
var re *regexp.Regexp

func Init_doproxy() {
	// read config file
	yfile, err := ioutil.ReadFile("hrp_config.yaml")
	if err != nil {
		log.Fatal(err)
	}

	data := make(map[interface{}]interface{})

	err2 := yaml.Unmarshal(yfile, &data)
	if err2 != nil {

		log.Fatal(err2)
	}

	for k, v := range data {
		fmt.Printf("%s -> %d\n", k, v)
	}

	// setup the proxy array
	proxies = make(map[string]proxy_service)
	// try this regexp to extract starting ~<username>(/....)
	re = regexp.MustCompile("^/~(.*?)(()|(/(.*)))$")
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
	//url := "http://web-www2019.astro.uni-bonn.de"
	url := "https://astro.uni-bonn.de/"
	np, err := NewProxy(url)

	if err != nil {
		log.Printf("Can't create proxy service for: %v (%v)", s, err.Error())
		return nil
	}

	// create a new entry
	pe := proxy_service{name: s, url: url, proxy: np, start: time.Now()}

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
	log.Printf("url-request: %v - %v - %v", r.URL.Path, r.RemoteAddr, r.Referer())

	// try to extract the username
	match := re.FindStringSubmatch(r.URL.Path)
	if match != nil {
		username := match[1]
		log.Printf("Extract username: %v", username)

		// check if we have already a defined proxy

		if pe, ok := proxies[username]; ok {
			log.Printf("Proxy for %v is available -> redirecting to %v", username, pe.url)
			pe.proxy.ServeHTTP(w, r)
			//http.NotFound(w, r)
		} else {
			log.Printf("Spwawning proxy for %v -> send temp page", username)
			send_wait_page(w, username)

			_ = create_proxy(username)
		}
	} else {
		http.NotFound(w, r)
	}

	//proxy.ServeHTTP(w, r)
	//http.NotFound(w, r)
}
