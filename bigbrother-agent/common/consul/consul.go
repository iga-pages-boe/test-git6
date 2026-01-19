package consul

import (
	"context"
	"net"
	"net/http"
	"os"
	"strings"

	bconsul "code.byted.org/gopkg/consul"
)

var IsBoeTest bool

func init() {
	IsBoeTest = os.Getenv("TCE_ENV") == "test"
}

func Lookup(name string) (bconsul.Endpoints, error) {
	endpoints, err := bconsul.Lookup(name)

	var filtered bconsul.Endpoints
	for i := range endpoints {
		if IsBoeTest {
			if endpoints[i].Tags["env"] == "test" {
				filtered = append(filtered, endpoints[i])
			}
		} else {
			if endpoints[i].Tags["env"] != "test" {
				filtered = append(filtered, endpoints[i])
			}
		}
	}
	return filtered, err
}

const clusterSep = "$"

var dialer = net.Dialer{}

func isIPAddr(s string) bool {
	h, _, _ := net.SplitHostPort(s)
	return h != "" && net.ParseIP(h) != nil
}

func ConsulHttpClient() *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if idx := strings.Index(addr, clusterSep); idx > 0 {
				addr = addr[:idx] // rm cluster name
			}
			if isIPAddr(addr) {
				return dialer.DialContext(ctx, network, addr)
			}
			name := addr
			if idx := strings.Index(addr, ":"); idx > 0 { // rm :80
				name = addr[:idx]
			}
			ee, err := Lookup(name)
			if err != nil {
				return nil, err
			}
			for tries := 3; ; tries-- {
				conn, err := dialer.DialContext(ctx, network, ee.GetOne().Addr)
				if err == nil {
					return conn, nil
				}
				if tries <= 0 || ctx.Err() != nil {
					return nil, err
				}
			}
		},
	}
	return &http.Client{
		Transport: transport,
	}
}
