package resolver_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const wantName = "testhost.testdomain"

func TestExampleCustomResolver(t *testing.T) {

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Hello World!", r.RemoteAddr, r.Proto)
	}))
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	client := ts.Client()
	transport, ok := ts.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatalf("failed to assert *http.Transport")
	}

	transport.DisableKeepAlives = false
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		fmt.Println("DIALING", network, addr)
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}

	// Connect directly to the listener
	url, err := url.Parse(ts.URL)
	if err != nil {
		panic(err)
	}
	for i := 0; i < 100; i++ {
		go func() {
			if err := connect(client, url.String()); err != nil {
				panic(err)
			}
		}()
	}

	if err := connect(client, url.String()); err != nil {
		panic(err)
	}

	// Output:
	// Got response 200 from http://127.0.0.1:44997: Hello World!
	// Got response 200 from http://testhost.testdomain:44997: Hello World!
}

func myLookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	// fqdn appends a dot
	if wantName == strings.TrimSuffix(host, ".") {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	return net.DefaultResolver.LookupIP(ctx, network, host)

}

func connect(client *http.Client, url string) error {
	resp, err := client.Get(url)
	if err != nil {
		log.Fatalf("Failed get: %s", err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf(
		"Got response %d from %s: %s\n",
		resp.StatusCode, url, string(body))
	return nil
}
