// Package netclient makes the app's HTTP clients. One way out serves them
// all: the listener's, which is direct, through the proxy the system's
// settings name as browsers go, or through one they give here. It is read as
// each request is made, so a change applies to the requests that follow.
package netclient

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/netproxy"
)

// The ways out.
const (
	// System sends requests through the proxy the system's settings name for
	// them, with its exceptions and setup script, or directly where they name
	// none: HTTP_PROXY, HTTPS_PROXY, ALL_PROXY and NO_PROXY, where set, decide
	// instead, as package netproxy has it. It's the way out unless the
	// listener chose another.
	System = "system"
	// Custom sends them through the proxy the listener gave, which ParseURL
	// reads.
	Custom = "custom"
	// Direct sends them to their hosts through no proxy at all.
	Direct = "direct"
)

// ParseURL reads the address of a proxy as the listener writes one: a host and
// port, such as 127.0.0.1:7890, which is an HTTP proxy, or one with its scheme,
// such as http://127.0.0.1:7890 or socks5://127.0.0.1:1080.
func ParseURL(address string) (*url.URL, error) {
	s := strings.TrimSpace(address)
	if s == "" {
		return nil, errors.New("no address")
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	case "socks5", "socks5h":
		u.Scheme = "socks5"
	default:
		return nil, fmt.Errorf("a proxy goes over http, https or socks5, not %s", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("no host to reach")
	}
	u.RawQuery, u.Fragment = "", ""
	u.Path, u.RawPath = "", ""
	return u, nil
}

// Set makes every request from now on go the way mode says: System, Custom
// through address, or Direct. It answers an error for a mode or an address no
// proxy goes by, and then leaves the way out as it was.
func Set(mode, address string) error {
	rt, err := transportFor(mode, address)
	if err != nil {
		return err
	}
	out.set(mode, address, rt)
	return nil
}

// wayOf is the way out a setting names: a setting never made is the system's.
func wayOf(mode string) string {
	if mode == "" {
		return System
	}
	return mode
}

// transportFor makes a transport going the way mode says. Each one of its own:
// Go keeps connections by the proxy they went through, and a transport that
// changed its proxy would keep the old ones for the new.
func transportFor(mode, address string) (*http.Transport, error) {
	t := http.DefaultTransport.(*http.Transport).Clone()
	switch wayOf(mode) {
	case Direct:
		t.Proxy = nil
	case Custom:
		u, err := ParseURL(address)
		if err != nil {
			return nil, err
		}
		t.Proxy = http.ProxyURL(u)
	case System:
		t.Proxy = netproxy.Proxy
	default:
		return nil, fmt.Errorf("no way out named %s", mode)
	}
	return t, nil
}

// Transport sends the requests of a client the listener's way out, whichever
// it is by the time each is made: share it between clients, as the app does.
func Transport() http.RoundTripper { return &out }

// Client is an HTTP client like http.Client with timeout, sending its requests
// the listener's way out. A timeout of 0 keeps none.
func Client(timeout time.Duration) *http.Client {
	return &http.Client{Transport: &out, Timeout: timeout}
}

// out is the one way out: it hands each request to the transport of the way
// out the settings last chose.
var out = router{mode: System}

type router struct {
	mu      sync.RWMutex
	mode    string
	address string
	rt      *http.Transport // nil until the first request asks
}

func (r *router) RoundTrip(req *http.Request) (*http.Response, error) {
	return r.transport().RoundTrip(req)
}

func (r *router) transport() *http.Transport {
	r.mu.RLock()
	rt := r.rt
	r.mu.RUnlock()
	if rt != nil {
		return rt
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rt == nil {
		// Going the system's way, as the first request of a setting never
		// made does, as in tests.
		r.rt, _ = transportFor(r.mode, r.address)
	}
	return r.rt
}

// set goes the way of rt from now on, and lets go of the connections the way
// before left idle.
func (r *router) set(mode, address string, rt *http.Transport) {
	r.mu.Lock()
	if wayOf(r.mode) == wayOf(mode) && strings.TrimSpace(r.address) == strings.TrimSpace(address) {
		r.mu.Unlock()
		return // already there: its connections stay
	}
	old := r.rt
	r.mode, r.address, r.rt = mode, address, rt
	r.mu.Unlock()
	if old != nil {
		old.CloseIdleConnections()
	}
}
