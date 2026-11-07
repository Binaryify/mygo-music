package app

import (
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeHeaderer is a plugin with headers for its covers.
type fakeHeaderer struct {
	hosts   map[string]bool // reached, by whether it names them; "*" for any
	headers map[string]string
	maxAge  time.Duration
	asked   atomic.Int32
	wait    chan struct{} // what an answer waits on, if anything
}

func (f *fakeHeaderer) Can(what string) bool { return what == "imageHeaders" }

func (f *fakeHeaderer) Reaches(host string) (bool, bool) {
	if f.hosts[host] {
		return true, true
	}
	return f.hosts["*"], false
}

func (f *fakeHeaderer) ImageHeaders(string) (map[string]string, time.Duration, error) {
	f.asked.Add(1)
	if f.wait != nil {
		<-f.wait
	}
	return f.headers, f.maxAge, nil
}

func coverURL(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}

// A plugin naming a host gives its covers' headers before one allowed any,
// and a host neither reaches is asked of no plugin.
func TestCoverHeadersAskNamedPluginFirst(t *testing.T) {
	wild := &fakeHeaderer{hosts: map[string]bool{"*": true}, headers: map[string]string{"Cookie": "any"}}
	named := &fakeHeaderer{hosts: map[string]bool{"music.local": true}, headers: map[string]string{"Cookie": "named"}}
	h := newCoverHeaders()
	h.setPlugins([]imageHeaderer{wild, named})
	if got := h.get(coverURL("https://music.local/c"), false); got["Cookie"] != "named" {
		t.Errorf("got %v", got)
	}
	if wild.asked.Load() != 0 {
		t.Error("asked the plugin allowed any host, after one naming it answered")
	}
	alone := newCoverHeaders()
	alone.setPlugins([]imageHeaderer{named})
	if got := alone.get(coverURL("https://other.example/c"), false); got != nil || named.asked.Load() != 1 {
		t.Errorf("got %v from a plugin not reaching the host", got)
	}
}

// A host's headers are asked once for as long as they hold, however many
// covers load at once, and anew when they've run out or the server turned
// them down, though not again so soon after.
func TestCoverHeadersCachedPerHost(t *testing.T) {
	p := &fakeHeaderer{hosts: map[string]bool{"music.local": true}, headers: map[string]string{"Cookie": "a"}, maxAge: time.Minute, wait: make(chan struct{})}
	now := time.Unix(1e9, 0)
	h := newCoverHeaders()
	h.now = func() time.Time { return now }
	h.setPlugins([]imageHeaderer{p})
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.get(coverURL("https://music.local/c?id="+string(rune('0'+i))), false)
		}()
	}
	time.Sleep(10 * time.Millisecond)
	close(p.wait)
	wg.Wait()
	if n := p.asked.Load(); n != 1 {
		t.Fatalf("asked %d times for covers loading at once", n)
	}
	now = now.Add(time.Second)
	if h.get(coverURL("https://music.local/c"), true); p.asked.Load() != 1 {
		t.Error("asked again just after asking, when turned down")
	}
	now = now.Add(coverHeadersAgain)
	if h.get(coverURL("https://music.local/c"), true); p.asked.Load() != 2 {
		t.Error("didn't ask again when turned down")
	}
	now = now.Add(time.Minute + time.Second)
	if h.get(coverURL("https://music.local/c"), false); p.asked.Load() != 3 {
		t.Error("didn't ask again once the headers ran out")
	}
}
