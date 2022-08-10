package pingpong

import (
	"encoding/json"
	"log"
	"net/http"
)

// written by: Oliver Cordes 2022-08-10
// changed by: Oliver Cordes 2022-08-10

var version string = "inf"

func Set_version(v string) {
	version = v
}

func Handle_ping_request(w http.ResponseWriter, r *http.Request) {
	log.Printf("ping: %v - %v - %v", r.URL.Path, r.RemoteAddr, r.Referer())

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	resp := make(map[string]string)
	resp["message"] = "pong"
	resp["version"] = version
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		log.Fatalf("Error happened in JSON marshal. Err: %s", err)
	}
	w.Write(jsonResp)
}
