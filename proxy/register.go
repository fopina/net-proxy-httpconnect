package proxy

import "golang.org/x/net/proxy"

// RegisterSchemes registers HTTPCONNECT dialer as proxy scheme handler for HTTP and HTTPS schemes
func RegisterSchemes() {
	// init() would be a good place to put this, but module might never be imported
	// such as code simply calling `golang.org/x/net/proxy.FromEnvironment()`
	proxy.RegisterDialerType("http", HTTPCONNECT)
	proxy.RegisterDialerType("https", HTTPCONNECT)
}
