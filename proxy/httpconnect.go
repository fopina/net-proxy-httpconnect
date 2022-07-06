package proxy

import (
	"errors"
	"log"
	"net/http"
	"net/url"

	"github.com/fopina/net-proxy-httpconnect/httpconnect"
	"golang.org/x/net/proxy"
)

// HTTPCONNECT returns a Dialer that makes HTTP CONNECT connections to the given address
func HTTPCONNECT(network string, address string, forward proxy.Dialer) (proxy.Dialer, error) {
	url, err := url.Parse(address)
	if err != nil {
		log.Fatalf("unable to parse address: %v", err)
	}

	transport := http.DefaultTransport.(*http.Transport)
	if forward != nil {
		transport.Dial = forward.Dial
	}

	if url.Scheme != "http" && url.Scheme != "https" {
		return nil, errors.New("Unsupported scheme: " + url.Scheme)
	}
	d := httpconnect.NewDialer(network, url, transport)
	return d, nil
}
