package doproxy

import (
	"html/template"
	"log"
	"net/http"
)

// documentations
// https://www.alexedwards.net/blog/serving-static-sites-with-go

type proxy_service struct {
	name string
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

func Handle_proxy_request(w http.ResponseWriter, r *http.Request) {
	log.Printf("url-request: %v - %v - %v", r.URL, r.RemoteAddr, r.Referer())
	//proxy.ServeHTTP(w, r)
	//http.NotFound(w, r)
	send_wait_page(w, "blubber")
}
