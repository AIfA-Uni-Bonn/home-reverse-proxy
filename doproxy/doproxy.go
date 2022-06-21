package doproxy

// written by: Oliver Cordes 2022-06-17
// changed by: Oliver Cordes 2022-06-21

import (
	"context"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"gopkg.in/yaml.v2"
)

// documentations
// https://www.alexedwards.net/blog/serving-static-sites-with-go

type proxy_service struct {
	name  string
	url   string
	proxy *httputil.ReverseProxy
	start time.Time
	// statistics
	last  time.Time // time of last call
	count int64     // number of calls
}

var Server_port int = 8080
var Debug bool = false
var proxies map[string]proxy_service
var re *regexp.Regexp
var proxy_network string = ""
var docker *client.Client

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

	if Debug {
		for k, v := range data {
			fmt.Printf("%s -> %d\n", k, v)
		}
	}

	// walk through the yaml structure, need to check group by group
	if _, ok := data["network"]; ok {
		// network is defined
		if n, ok2 := data["network"].(map[interface{}]interface{})["name"]; ok2 {
			//if n, ok2 := d["name"]; ok2 {
			proxy_network = n.(string)
		}
	}

	if d, ok := data["port"]; ok {
		Server_port = d.(int)
	}

	if d, ok := data["debug"]; ok {
		Debug = d.(bool)
	}

	if proxy_network != "" {
		log.Printf("host network: %v", proxy_network)
	} else {
		log.Println("Not network configured!")
	}

	// setup the proxy array
	proxies = make(map[string]proxy_service)
	// try this regexp to extract starting ~<username>(/....)
	re = regexp.MustCompile("^/~(.*?)(()|(/(.*)))$")

	// start the docker client
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Fatalln(err)
	}
	// copy the client variable
	docker = cli

	err = CreateNetwork(proxy_network)

	if err != nil {
		log.Fatalln(err)
	}
}

func CreateNetwork(network_name string) error {
	networks, err := docker.NetworkList(context.Background(), types.NetworkListOptions{})
	if err != nil {
		return err
	}

	// search for the host network for the reverse proxy container
	for _, network := range networks {
		if network.Name == network_name {
			log.Printf("Host network '%s' is available!", network_name)
			return nil
		}
	}
	// the network is available
	options := types.NetworkCreate{
		CheckDuplicate: true,
		Driver:         "bridge",
	}

	log.Printf("Create missing host network: %s", network_name)
	_, err = docker.NetworkCreate(context.Background(), network_name, options)

	return err
}

func TestExistingContainer(username string) (string, error) {
	// get all running containers
	containers, err := docker.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		log.Printf("Can't read the list of running containers (%v)\n", err)
		return "", err
	}
	// name must begin with / in contrast to the create process,
	// which omits the /, don't know why!
	name := fmt.Sprintf("/userwebsite_%s", username)
	for _, container := range containers {
		if container.Names[0] == name {
			log.Printf("Container '%s' found!", name)
			data, _ := docker.ContainerInspect(context.Background(), container.ID)

			// extract the IP address depending on the network settings
			var ip_addr string
			if proxy_network == "" {
				ip_addr = data.NetworkSettings.IPAddress
			} else {
				ip_addr = data.NetworkSettings.Networks["proxy"].IPAddress
			}

			return ip_addr, nil
		}
	}

	return "", nil
}

func SpawnContainer(username string) (string, error) {
	// check if container is already running
	ip_addr, err := TestExistingContainer(username)

	if ip_addr != "" {
		return ip_addr, nil
	}

	// mounts
	mounts := []mount.Mount{
		{
			Type:   "bind",
			Source: "/Users/ocordes/volatile/public_html",
			Target: fmt.Sprintf("/users/%s/public_html", username),
		},
	}

	// host config
	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{
			Name: "always",
		},
		Mounts: mounts,
	}

	// https://godoc.org/github.com/docker/docker/api/types/network#NetworkingConfig
	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{},
	}
	//gatewayConfig := &network.EndpointSettings{
	//	Gateway: "172.20.0.1",
	//}
	//networkConfig.EndpointsConfig[proxy_network] = gatewayConfig
	networkConfig.EndpointsConfig[proxy_network] = &network.EndpointSettings{}

	name := fmt.Sprintf("userwebsite_%s", username)

	config := &container.Config{
		Image:        "registry.gitlab.com/ocordes/userwebsite",
		Env:          []string{fmt.Sprintf("USERNAME=%s", username)},
		ExposedPorts: nil,
		Hostname:     name,
	}

	container, err := docker.ContainerCreate(context.Background(), config, hostConfig, networkConfig, nil, name)

	if err != nil {
		log.Printf("Error spawning new container: %v", err)
		return "", err
	}

	// Run the actual container
	docker.ContainerStart(context.Background(), container.ID, types.ContainerStartOptions{})
	log.Printf("Container for user %s is created: %s\n", username, container.ID)

	data, err := docker.ContainerInspect(context.Background(), container.ID)

	// extract the IP address depending on the network settings
	if proxy_network == "" {
		ip_addr = data.NetworkSettings.IPAddress
	} else {
		ip_addr = data.NetworkSettings.Networks["proxy"].IPAddress
	}

	return ip_addr, nil
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

	proxy.ModifyResponse = modifyResponse
	proxy.ErrorHandler = errorHandler()
	return proxy, nil
}

// handler modifying responses
func modifyResponse(res *http.Response) error {
	log.Printf("%v -> %v", res.Status, res.Request.URL)
	return nil
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

	// spawn continer
	ip_addr, err := SpawnContainer(s)

	if err != nil {
		log.Printf("Can't create proxy service for:  %v (%v)", s, err.Error())
		return nil
	}

	url := fmt.Sprintf("http://%s/", ip_addr)
	np, err := NewProxy(url)

	if err != nil {
		log.Printf("Can't create proxy service for: %v (%v)", s, err.Error())
		return nil
	}

	// create a new entry
	pe := proxy_service{name: s, url: url, proxy: np, start: time.Now(), count: 0}

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
	if Debug {
		log.Printf("url-request: %v - %v - %v", r.URL.Path, r.RemoteAddr, r.Referer())
	}

	// try to extract the username
	match := re.FindStringSubmatch(r.URL.Path)
	if match != nil {
		username := match[1]
		if Debug {
			log.Printf("Extract username: %v", username)
		}

		// check if we have already a defined proxy
		if pe, ok := proxies[username]; ok {
			if Debug {
				log.Printf("Proxy for %v is available -> redirecting to %v", username, pe.url)
			}
			pe.count += 1
			pe.last = time.Now()
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
