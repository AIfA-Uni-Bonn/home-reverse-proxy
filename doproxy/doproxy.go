package doproxy

// written by: Oliver Cordes 2022-06-17
// changed by: Oliver Cordes 2022-07-01

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/user"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/go-ldap/ldap/v3"
	"gopkg.in/yaml.v2"
)

// documentations
// https://www.alexedwards.net/blog/serving-static-sites-with-go

type proxy_service struct {
	name         string
	ready        bool
	url          string
	proxy        *httputil.ReverseProxy
	start        time.Time
	container_id string
	// statistics
	last  time.Time // time of last call
	count int64     // number of calls
}

var Server_port int = 8080
var Debug bool = false
var proxies map[string]proxy_service
var re *regexp.Regexp

var info_func func(string) ([]string, error)

// docker components
var docker *client.Client
var docker_image string = "registry.gitlab.com/ocordes/userwebsite:latest"
var docker_network string = ""

// culling components
var Culling bool = false
var Culling_every int = 600
var Culling_timeout int = 600

// ldap components
var ldap_server string = ""
var ldap_base string = ""
var ldap_user_attr string = ""
var ldap_directories_attr string = ""

// helper functions
func extract_username(re *regexp.Regexp, s string) string {
	// try to extract the username
	match := re.FindStringSubmatch(s)
	if match != nil {
		username := match[1]
		if Debug {
			log.Printf("Extract username: %v", username)
		}
		return username
	} else {
		return ""
	}
}

// initialize the module/package
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

	if d, ok := data["info"]; ok {
		s := d.(string)
		switch s {
		case "ldap":
			log.Printf("Using ldap connection!")
			info_func = GetLdapInfos
		case "passwd":
			log.Printf("Using os password file!")
			info_func = GetPasswdInfos
		default:
			log.Printf("Unknown info type '%s' given, only passwd|ldap are allowd", s)
			log.Printf("Using os password file!")
			info_func = GetPasswdInfos
		}
	}

	// walk through the yaml structure, need to check group by group
	if _, ok := data["cull"]; ok {
		// cull is defined
		if _, ok2 := data["cull"].(map[interface{}]interface{})["enabled"]; ok2 {
			Culling = true
		}
		if n, ok2 := data["cull"].(map[interface{}]interface{})["every"]; ok2 {
			Culling_every = n.(int)
		}
		if n, ok2 := data["cull"].(map[interface{}]interface{})["timeout"]; ok2 {
			Culling_timeout = n.(int)
		}
	}

	if _, ok := data["docker"]; ok {
		// cull is defined
		if n, ok2 := data["docker"].(map[interface{}]interface{})["image"]; ok2 {
			docker_image = n.(string)
		}
		if n, ok2 := data["docker"].(map[interface{}]interface{})["network"]; ok2 {
			docker_network = n.(string)
		}

	}

	if _, ok := data["ldap"]; ok {
		// ldap is defined
		if n, ok2 := data["ldap"].(map[interface{}]interface{})["server"]; ok2 {
			ldap_server = n.(string)
			log.Printf("Using LDAP service: %s", ldap_server)
		}
		if n, ok2 := data["ldap"].(map[interface{}]interface{})["base"]; ok2 {
			ldap_base = n.(string)
			log.Printf("Using LDAP base: %s", ldap_base)
		}
		if n, ok2 := data["ldap"].(map[interface{}]interface{})["user_attr"]; ok2 {
			ldap_user_attr = n.(string)
			log.Printf("Using LDAP user-identifier: %s", ldap_user_attr)
		}
		if n, ok2 := data["ldap"].(map[interface{}]interface{})["directories_attr"]; ok2 {
			ldap_directories_attr = n.(string)
			log.Printf("Using LDAP directories-identifier: %s", ldap_directories_attr)
		}
	}

	if d, ok := data["port"]; ok {
		Server_port = d.(int)
	}

	if d, ok := data["debug"]; ok {
		Debug = d.(bool)
	}

	if docker_network != "" {
		log.Printf("Host network: %v", docker_network)
	} else {
		log.Println("Not host network configured!")
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

	err = CreateNetwork(docker_network)

	if err != nil {
		log.Fatalln(err)
	}
}

// Service_culling
//
// looks over the proxy list and removes every proxy which has called
// the last time before the timeout limit

func Service_culling() {
	log.Printf("Culling service started ...")
	for username, value := range proxies {
		tdiff := time.Now().Sub(value.last).Seconds()
		log.Printf("%s: count=%v last=%v s container_id=%v", username, value.count, tdiff, value.container_id)
		if tdiff > float64(Culling_timeout) {
			log.Printf("Removing proxy for '%s' ...", username)
			err := RemoveContainer(username, value.container_id)
			if err != nil {
				log.Printf("Removing container for '%s' failed (%v)", username, err.Error())
			}

			// remove, even if the docker container failed to be removed, all other reactions
			// will throw an error while reattaching ;-)
			delete(proxies, username)

		}
	}
	log.Printf("Culling service finished!")
}

// Service_deep_culling
//
// looks inside the docker container list to look for containers which are started by a previous instance
// and are not handled by the proxy list
func Service_deep_culling() {
	log.Printf("Deep culling service started ...")

	containers, err := docker.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		log.Printf("Can't read the list of running containers (%v)\n", err)
		return
	}

	re := regexp.MustCompile("^/userwebsite_(.*?)$")

	for _, container := range containers {
		// extract the username from the container name
		match := re.FindStringSubmatch(container.Names[0])
		if match != nil {
			username := match[1]
			if Debug {
				log.Printf("webpage container found: %s", username)
			}

			// checks if container is in the proxy list
			if _, ok := proxies[username]; ok {
				if Debug {
					log.Printf("Webpage container for '%s' is supported!", username)
				}
			} else {
				err := RemoveContainer(username, container.ID)
				if err != nil {
					log.Printf("Removing container for '%s' failed (%v)", username, err.Error())
				} else {
					log.Printf("Removing container for '%s' while not used!", username)
				}
			}
		}

	}

	log.Printf("Deep culling service finished!")
}

// ldap related functions
func GetLdapInfos(username string) ([]string, error) {
	var directories []string

	log.Printf("Connecting to ldap...")

	l, err := ldap.DialURL(ldap_server)
	if err != nil {
		return directories, err
	}
	defer l.Close()

	attributes := []string{"dn", "cn", "homeDirectory"}

	if ldap_directories_attr != "" {
		attributes = append(attributes, ldap_directories_attr)
	}

	searchRequest := ldap.NewSearchRequest(
		ldap_base, // The base dn to search
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(%s=%s)", ldap_user_attr, username),
		attributes, // a list of attributes to retrieve
		//[]string{"dn", "cn", "authorizedService", "homeDirectory"}, // A list attributes to retrieve
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return directories, err
	}

	if len(sr.Entries) == 0 {
		newerr := errors.New("User not found!")
		return directories, newerr
	}

	entry := sr.Entries[0]

	if Debug {
		log.Printf("LDAP-Info: dn:%s, cn:%v, %v\n", entry.DN, entry.GetAttributeValue("cn"),
			entry.GetAttributeValue("homeDirectory"))

		if ldap_directories_attr != "" {
			log.Printf("LDAP-Info: directories=%v\n", entry.GetAttributeValues(ldap_directories_attr))
		}
	}

	// check if everything is OK

	// check for home directory
	homedir := entry.GetAttributeValue("homeDirectory") + "/public_html"

	directories = append(directories, homedir)

	if ldap_directories_attr != "" {
		for _, dir := range entry.GetAttributeValues(ldap_directories_attr) {
			directories = append(directories, dir)
		}
	}

	log.Printf("ldap info complete!")

	return directories, nil
}

// Passwd related functions
func GetPasswdInfos(username string) ([]string, error) {
	var directories []string

	user_info, err := user.Lookup(username)

	if err != nil {
		return directories, err
	}

	directories = append(directories, user_info.HomeDir+"/public_html")

	return directories, nil
}

// docker related functions

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

func TestExistingContainer(username string) (string, string, error) {
	// get all running containers
	containers, err := docker.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		log.Printf("Can't read the list of running containers (%v)\n", err)
		return "", "", err
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
			if docker_network == "" {
				ip_addr = data.NetworkSettings.IPAddress
			} else {
				ip_addr = data.NetworkSettings.Networks[docker_network].IPAddress
			}

			return ip_addr, container.ID, nil
		}
	}

	return "", "", nil
}

func RemoveContainer(username string, container_id string) error {
	err := docker.ContainerStop(context.Background(), container_id, nil)
	if err != nil {
		return err
	}
	err = docker.ContainerRemove(context.Background(), container_id, types.ContainerRemoveOptions{})
	return err
}

func CheckHomedirectory(username string, directory string, mounts []mount.Mount) ([]mount.Mount, error) {
	_, err := os.Stat(directory)

	if err != nil {
		return mounts, err
	} else {
		// is it necessary to check, if the directory is a directory?
		//log.Printf("%v", finfo)
		//log.Printf("%v", finfo.IsDir())
		//log.Printf("%s", finfo.Name())
		m := mount.Mount{
			Type:     "bind",
			Source:   directory,
			Target:   fmt.Sprintf("/users/%s/public_html", username),
			ReadOnly: false,
		}
		mounts = append(mounts, m)
	}

	return mounts, nil
}

func CheckAdditionalDirectories(directories []string, mounts []mount.Mount) []mount.Mount {
	for _, dir := range directories {
		s := strings.Split(dir, "::")
		if Debug {
			log.Printf("%s\n", dir)
			log.Printf("%v\n", s)
		}
		// s[0] is the directory, s[1] is the readonly flag (if available)
		is_ro := false
		if len(s) > 1 {
			is_ro = s[1] == "ro"
		}

		// check if directory is available
		_, err := os.Stat(s[0])

		if err != nil {
			log.Printf("%s not found! (%v)", s[0], err.Error())
		} else {
			m := mount.Mount{
				Type:     "bind",
				Source:   s[0],
				Target:   s[0],
				ReadOnly: is_ro,
			}
			mounts = append(mounts, m)
		}
	}

	return mounts
}

func SpawnContainer(username string) (string, string, error) {
	// check if container is already running
	ip_addr, container_id, _ := TestExistingContainer(username)

	if ip_addr != "" {
		return ip_addr, container_id, nil
	}

	//var dirs []string
	//var err error

	//switch info_mode {
	//case info_ldap:
	//	dirs, err = GetLdapInfos(username)
	//case info_passwd:
	//	dirs, err = GetPasswdInfos(username)
	//default:
	//	dirs, err = GetPasswdInfos(username)
	//}

	dirs, err := info_func(username)

	if err != nil {
		log.Printf("LDAP-Error: %v", err.Error())
		return "", "", err
	}

	fmounts := []mount.Mount{}

	fmounts, err = CheckHomedirectory(username, dirs[0], fmounts)
	if err != nil {
		return "", "", err
	}

	// add additional directories to the mount array
	if len(dirs) > 1 {
		fmounts = CheckAdditionalDirectories(dirs[1:], fmounts)
	}

	// mounts
	//mounts := []mount.Mount{
	//	{
	//		Type:   "bind",
	//		Source: "/Users/ocordes/volatile/public_html",
	//		Target: fmt.Sprintf("/users/%s/public_html", username),
	//	},
	//}

	// host config
	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{
			Name: "always",
		},
		Mounts: fmounts,
	}

	// https://godoc.org/github.com/docker/docker/api/types/network#NetworkingConfig
	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{},
	}
	//gatewayConfig := &network.EndpointSettings{
	//	Gateway: "172.20.0.1",
	//}
	//networkConfig.EndpointsConfig[proxy_network] = gatewayConfig
	networkConfig.EndpointsConfig[docker_network] = &network.EndpointSettings{}

	name := fmt.Sprintf("userwebsite_%s", username)

	config := &container.Config{
		//Image:        "registry.gitlab.com/ocordes/userwebsite",
		Image:        docker_image,
		Env:          []string{fmt.Sprintf("USERNAME=%s", username)},
		ExposedPorts: nil,
		Hostname:     name,
	}

	container, err := docker.ContainerCreate(context.Background(), config, hostConfig, networkConfig, nil, name)

	if err != nil {
		log.Printf("Error spawning new container: %v", err)
		return "", "", err
	}

	// Run the created container
	docker.ContainerStart(context.Background(), container.ID, types.ContainerStartOptions{})
	log.Printf("Container for user %s is created: %s\n", username, container.ID)

	data, _ := docker.ContainerInspect(context.Background(), container.ID)

	// extract the IP address depending on the network settings
	if docker_network == "" {
		ip_addr = data.NetworkSettings.IPAddress
	} else {
		ip_addr = data.NetworkSettings.Networks[docker_network].IPAddress
	}

	return ip_addr, container.ID, nil
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
	log.Printf("%v -> %v (%v)", res.Status, res.Request.URL, res.Request.RemoteAddr)
	return nil
}

// handler for modifying requests
func modifyRequest(req *http.Request) {
	req.Header.Set("X-Proxy", "Simple-Reverse-Proxy")
}

// error handler requests (after calling the proxy)
//
// if the final proxy cannot be called, this routine will be
// called, it will remove the proxy from the list and triggers
// a reload with the wait page, if something strange is happening
// the spawning methods take over the error handling
func errorHandler() func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, req *http.Request, err error) {
		log.Printf("Got error while modifying response: %v \n", err)

		// remove user from proxy list
		username := extract_username(re, req.URL.Path)
		delete(proxies, username)
		// trigger a reload of the proxy
		send_wait_page(w, username)
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

func create_proxy(s string) error {
	// spawn continer
	ip_addr, container_id, err := SpawnContainer(s)

	if err != nil {
		log.Printf("Can't create proxy service for:  %v (%v)", s, err.Error())
		return err
	}

	url := fmt.Sprintf("http://%s/", ip_addr)
	np, err := NewProxy(url)

	if err != nil {
		log.Printf("Can't create proxy service for: %v (%v)", s, err.Error())
		return err
	}

	// update the proxy entry
	pe := proxies[s]

	// create a new entry
	//pe := proxy_service{name: s, url: url, proxy: np, container_id: container_id, start: time.Now(), count: 0, last: time.Now()}
	pe.url = url
	pe.proxy = np
	pe.container_id = container_id
	pe.start = time.Now()
	pe.count = 0
	pe.last = time.Now()
	pe.ready = true

	proxies[s] = pe

	return nil
}

// Handle_proxy_request
//
// works as the central station of the reverse proxy,
// checks, if a proxy is still running, or create a new proxy
// call the proxy if available, a creation may refer to a temp
// web page with an automatic reload! docker needs a few seconds
// to start the container properly
// -> during startup the browser needs to wait, because we check
// all necessary parts ...
func Handle_proxy_request(w http.ResponseWriter, r *http.Request) {
	if Debug {
		log.Printf("url-request: %v - %v - %v", r.URL.Path, r.RemoteAddr, r.Referer())
	}

	username := extract_username(re, r.URL.Path)
	if username != "" {
		// check if we have already a defined proxy
		if pe, ok := proxies[username]; ok {
			if pe.ready == false {
				// the proxy was called before the container was ready
				log.Printf("Proxy for %v is starting -> send wait page!", username)
				send_wait_page(w, username)
			} else {
				if Debug {
					log.Printf("Proxy for %v is available -> redirecting to %v", username, pe.url)
				}
				pe.count += 1
				pe.last = time.Now()
				proxies[username] = pe
				pe.proxy.ServeHTTP(w, r)
				//http.NotFound(w, r)
			}
		} else {
			log.Printf("Spwawning proxy for '%v' (%v) ...", username, r.URL.Path)

			// create a new proxy entry, marking the container as not ready
			proxy := proxy_service{name: username, ready: false}
			proxies[username] = proxy

			err := create_proxy(username)
			if err != nil {
				// remove proxy from list
				delete(proxies, username)
				http.Error(w, http.StatusText(500), 500)
				log.Printf("Spawning aborted!")
			} else {
				send_wait_page(w, username)
				log.Printf("Spawning complete!")
			}
		}
	} else {
		http.NotFound(w, r)
	}

	//proxy.ServeHTTP(w, r)
	//http.NotFound(w, r)
}
