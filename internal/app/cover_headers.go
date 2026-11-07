package app

import (
	"net/url"
	"strings"
	"sync"
	"time"
)

// imageHeaderer is a plugin as the cover headers ask it, which the tests fake.
type imageHeaderer interface {
	Can(what string) bool
	Reaches(host string) (reaches, named bool)
	ImageHeaders(url string) (map[string]string, time.Duration, error)
}

const (
	// coverHeadersAge is how long a plugin's headers hold for a host when it
	// didn't say.
	coverHeadersAge = 5 * time.Minute
	// coverHeadersAgain is how soon after asking a host's headers are asked
	// again when the server turned them down, so a cover it never serves
	// doesn't have the plugin asked for every load.
	coverHeadersAgain = 10 * time.Second
)

// coverHeaders asks the plugins that are on for the headers of the covers of
// a host, such as the cookie of the session a source keeps them behind: those
// whose network permission names the host first, then those allowed any. The
// first that has some gives them, for as long as it says. Covers keep their
// URLs, which the songs saved keep: the headers, which may run out, are only
// ever asked for as a cover loads.
type coverHeaders struct {
	mu      sync.Mutex
	plugins []imageHeaderer
	hosts   map[string]*hostHeaders
	now     func() time.Time
}

// hostHeaders are what the plugins answered for a host, or will.
type hostHeaders struct {
	ready   chan struct{} // closed once they're asked
	headers map[string]string
	asked   time.Time
	until   time.Time
}

func newCoverHeaders() *coverHeaders {
	return &coverHeaders{hosts: map[string]*hostHeaders{}, now: time.Now}
}

// setPlugins takes the plugins that are on, forgetting what any answered.
func (h *coverHeaders) setPlugins(plugins []imageHeaderer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.plugins = plugins
	h.hosts = map[string]*hostHeaders{}
}

// get answers the headers of the cover at u, asking for them anew if stale,
// as after the server turned down those it answered before.
func (h *coverHeaders) get(u *url.URL, stale bool) map[string]string {
	host := strings.ToLower(u.Hostname())
	h.mu.Lock()
	e := h.hosts[host]
	now := h.now()
	if e != nil {
		select {
		case <-e.ready:
			if now.After(e.until) || stale && now.Sub(e.asked) >= coverHeadersAgain {
				e = nil
			} else if stale {
				// Just asked: the cover is turned down with what's fresh.
				h.mu.Unlock()
				return nil
			}
		default:
		}
	}
	if e != nil {
		h.mu.Unlock()
		<-e.ready
		return e.headers
	}
	var asking []imageHeaderer
	for _, named := range []bool{true, false} {
		for _, p := range h.plugins {
			if reaches, n := p.Reaches(host); reaches && n == named && p.Can("imageHeaders") {
				asking = append(asking, p)
			}
		}
	}
	if len(asking) == 0 {
		h.mu.Unlock()
		return nil
	}
	e = &hostHeaders{ready: make(chan struct{}), asked: now}
	h.hosts[host] = e
	h.mu.Unlock()

	age := coverHeadersAge
	for _, p := range asking {
		headers, maxAge, err := p.ImageHeaders(u.String())
		if err != nil || len(headers) == 0 {
			continue
		}
		e.headers = headers
		if maxAge > 0 {
			age = maxAge
		}
		break
	}
	e.until = now.Add(age)
	close(e.ready)
	return e.headers
}
