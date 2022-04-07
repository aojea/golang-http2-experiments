package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"

	"golang.org/x/net/http2"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// NewProxy takes target host and creates a reverse proxy
func NewProxy(targetHost string) (*httputil.ReverseProxy, error) {
	url, err := url.Parse(targetHost)
	if err != nil {
		return nil, err
	}

	log.Printf("Reversing proxy to %s", url)
	proxy := httputil.NewSingleHostReverseProxy(url)

	/*
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
				originalDirector(req)
				modifyRequest(req)
		}

		proxy.ModifyResponse = modifyResponse()
		proxy.ErrorHandler = errorHandler()
	*/
	return proxy, nil
}

func modifyRequest(req *http.Request) {
	req.Header.Set("X-Proxy", "Simple-Reverse-Proxy")
}

func errorHandler() func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, req *http.Request, err error) {
		fmt.Printf("Got error while modifying response: %v \n", err)
		return
	}
}

func modifyResponse() func(*http.Response) error {
	return func(resp *http.Response) error {
		return errors.New("response body is invalid")
	}
}

// ProxyRequestHandler handles the http request using proxy
func ProxyRequestHandler(proxy *httputil.ReverseProxy) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	}
}

func main() {

	// syncer -----> apiserver
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	kcphost := flag.String("kcp", "https://localhost:8000", "kcp endpoint url")
	kcpcert := flag.String("kcpcert", "server.crt", "kcp certificate file")
	flag.Parse()

	go server()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// initialize a reverse proxy and pass the actual backend server url here
	proxy, err := NewProxy(config.Host)
	if err != nil {
		panic(err)
	}
	transport, err := rest.TransportFor(config)
	if err != nil {
		panic(err)
	}
	proxy.Transport = transport

	// kcp  -----> syncer (reverse connection)
	client := &http.Client{}
	caCert, err := ioutil.ReadFile(*kcpcert)
	if err != nil {
		log.Fatalf("Reading server certificate: %s", err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	tlsConfig := &tls.Config{
		RootCAs:            caCertPool,
		InsecureSkipVerify: true,
	}
	client.Transport = &http2.Transport{
		TLSClientConfig: tlsConfig,
	}

	l := NewListener(client, *kcphost+"/revdial")
	go l.run()
	defer l.Close()
	// serve requests
	mux := http.NewServeMux()
	mux.HandleFunc("/", ProxyRequestHandler(proxy))
	server := &http.Server{Handler: mux}
	defer server.Close()
	log.Printf("Serving on Reverse connection")
	log.Fatal(server.Serve(l))
}

func server() {

	dialer := NewDialer()
	defer dialer.Close()

	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(dialer.ProxyRequestHandler()))
	mux.Handle("/revdial", dialer)

	// Create a server on port 8000
	// Exactly how you would run an HTTP/1.1 server
	srv := &http.Server{Addr: ":8000", Handler: mux}
	defer srv.Close()

	// Start the server with TLS, since we are running HTTP/2 it must be
	// run with TLS.
	// Exactly how you would run an HTTP/1.1 server with TLS connection.
	log.Printf("Serving on https://0.0.0.0:8000")
	log.Fatal(srv.ListenAndServeTLS("server.crt", "server.key"))
}
