package main

import (
	"crypto/tls"
	"crypto/x509"
	"log"
	"net/http"
)

// localhostCert was generated from crypto/tls/generate_cert.go with the following command:
//     go run generate_cert.go  --rsa-bits 2048 --host 127.0.0.1,::1,example.com --ca --start-date "Jan 1 00:00:00 1970" --duration=1000000h
var localhostCert = []byte(`-----BEGIN CERTIFICATE-----
MIIDGDCCAgCgAwIBAgIQTKCKn99d5HhQVCLln2Q+eTANBgkqhkiG9w0BAQsFADAS
MRAwDgYDVQQKEwdBY21lIENvMCAXDTcwMDEwMTAwMDAwMFoYDzIwODQwMTI5MTYw
MDAwWjASMRAwDgYDVQQKEwdBY21lIENvMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A
MIIBCgKCAQEA1Z5/aTwqY706M34tn60l8ZHkanWDl8mM1pYf4Q7qg3zA9XqWLX6S
4rTYDYCb4stEasC72lQnbEWHbthiQE76zubP8WOFHdvGR3mjAvHWz4FxvLOTheZ+
3iDUrl6Aj9UIsYqzmpBJAoY4+vGGf+xHvuukHrVcFqR9ZuBdZuJ/HbbjUyuNr3X9
erNIr5Ha17gVzf17SNbYgNrX9gbCeEB8Z9Ox7dVuJhLDkpF0T/B5Zld3BjyUVY/T
cukU4dTVp6isbWPvCMRCZCCOpb+qIhxEjJ0n6tnPt8nf9lvDl4SWMl6X1bH+2EFa
a8R06G0QI+XhwPyjXUyCR8QEOZPCR5wyqQIDAQABo2gwZjAOBgNVHQ8BAf8EBAMC
AqQwEwYDVR0lBAwwCgYIKwYBBQUHAwEwDwYDVR0TAQH/BAUwAwEB/zAuBgNVHREE
JzAlggtleGFtcGxlLmNvbYcEfwAAAYcQAAAAAAAAAAAAAAAAAAAAATANBgkqhkiG
9w0BAQsFAAOCAQEAThqgJ/AFqaANsOp48lojDZfZBFxJQ3A4zfR/MgggUoQ9cP3V
rxuKAFWQjze1EZc7J9iO1WvH98lOGVNRY/t2VIrVoSsBiALP86Eew9WucP60tbv2
8/zsBDSfEo9Wl+Q/gwdEh8dgciUKROvCm76EgAwPGicMAgRsxXgwXHhS5e8nnbIE
Ewaqvb5dY++6kh0Oz+adtNT5OqOwXTIRI67WuEe6/B3Z4LNVPQDIj7ZUJGNw8e6L
F4nkUthwlKx4yEJHZBRuFPnO7Z81jNKuwL276+mczRH7piI6z9uyMV/JbEsOIxyL
W6CzB7pZ9Nj1YLpgzc1r6oONHLokMJJIz/IvkQ==
-----END CERTIFICATE-----`)

// localhostKey is the private key for localhostCert.
var localhostKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA1Z5/aTwqY706M34tn60l8ZHkanWDl8mM1pYf4Q7qg3zA9XqW
LX6S4rTYDYCb4stEasC72lQnbEWHbthiQE76zubP8WOFHdvGR3mjAvHWz4FxvLOT
heZ+3iDUrl6Aj9UIsYqzmpBJAoY4+vGGf+xHvuukHrVcFqR9ZuBdZuJ/HbbjUyuN
r3X9erNIr5Ha17gVzf17SNbYgNrX9gbCeEB8Z9Ox7dVuJhLDkpF0T/B5Zld3BjyU
VY/TcukU4dTVp6isbWPvCMRCZCCOpb+qIhxEjJ0n6tnPt8nf9lvDl4SWMl6X1bH+
2EFaa8R06G0QI+XhwPyjXUyCR8QEOZPCR5wyqQIDAQABAoIBAFAJmb1pMIy8OpFO
hnOcYWoYepe0vgBiIOXJy9n8R7vKQ1X2f0w+b3SHw6eTd1TLSjAhVIEiJL85cdwD
MRTdQrXA30qXOioMzUa8eWpCCHUpD99e/TgfO4uoi2dluw+pBx/WUyLnSqOqfLDx
S66kbeFH0u86jm1hZibki7pfxLbxvu7KQgPe0meO5/13Retztz7/xa/pWIY71Zqd
YC8UckuQdWUTxfuQf0470lAK34GZlDy9tvdVOG/PmNkG4j6OQjy0Kmz4Uk7rewKo
ZbdphaLPJ2A4Rdqfn4WCoyDnxlfV861T922/dEDZEbNWiQpB81G8OfLL+FLHxyIT
LKEu4R0CgYEA4RDj9jatJ/wGkMZBt+UF05mcJlRVMEijqdKgFwR2PP8b924Ka1mj
9zqWsfbxQbdPdwsCeVBZrSlTEmuFSQLeWtqBxBKBTps/tUP0qZf7HjfSmcVI89WE
3ab8LFjfh4PtK/LOq2D1GRZZkFliqi0gKwYdDoK6gxXWwrumXq4c2l8CgYEA8vrX
dMuGCNDjNQkGXx3sr8pyHCDrSNR4Z4FrSlVUkgAW1L7FrCM911BuGh86FcOu9O/1
Ggo0E8ge7qhQiXhB5vOo7hiVzSp0FxxCtGSlpdp4W6wx6ZWK8+Pc+6Moos03XdG7
MKsdPGDciUn9VMOP3r8huX/btFTh90C/L50sH/cCgYAd02wyW8qUqux/0RYydZJR
GWE9Hx3u+SFfRv9aLYgxyyj8oEOXOFjnUYdY7D3KlK1ePEJGq2RG81wD6+XM6Clp
Zt2di0pBjYdi0S+iLfbkaUdqg1+ImLoz2YY/pkNxJQWQNmw2//FbMsAJxh6yKKrD
qNq+6oonBwTf55hDodVHBwKBgEHgEBnyM9ygBXmTgM645jqiwF0v75pHQH2PcO8u
Q0dyDr6PGjiZNWLyw2cBoFXWP9DYXbM5oPTcBMbfizY6DGP5G4uxzqtZHzBE0TDn
OKHGoWr5PG7/xDRrSrZOfe3lhWVCP2XqfnqoKCJwlOYuPws89n+8UmyJttm6DBt0
mUnxAoGBAIvbR87ZFXkvqstLs4KrdqTz4TQIcpzB3wENukHODPA6C1gzWTqp+OEe
GMNltPfGCLO+YmoMQuTpb0kECYV3k4jR3gXO6YvlL9KbY+UOA6P0dDX4ROi2Rklj
yh+lxFLYa1vlzzi9r8B7nkR9hrOGMvkfXF42X89g7lx4uMtu2I4q
-----END RSA PRIVATE KEY-----`)

func main() {
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(localhostCert) {
		log.Fatal("Failed to append Cert from PEM")
	}
	cert, err := tls.X509KeyPair(localhostCert, localhostKey)
	if err != nil {
		log.Fatalf("Failed to build cert with error: %+v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Add("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		w.Write([]byte("This is an example server.\n"))
		log.Printf("Connection req from %v", req.RemoteAddr)
	})
	cfg := &tls.Config{
		RootCAs:      roots,
		Certificates: []tls.Certificate{cert},
	}
	srv1 := &http.Server{
		Addr:      ":8443",
		Handler:   mux,
		TLSConfig: cfg,
	}
	log.Fatal(srv1.ListenAndServeTLS("", ""))
	srv2 := &http.Server{
		Addr:      ":9443",
		Handler:   mux,
		TLSConfig: cfg,
	}
	log.Fatal(srv2.ListenAndServeTLS("", ""))
}
