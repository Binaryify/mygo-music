package netclient

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// carrier is a server that answers as a proxy does, keeping the URL of each
// request it carries.
func carrier(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.String())
		mu.Unlock()
		io.WriteString(w, "carried")
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

// get fetches url, as the app would, and answers what came back.
func get(t *testing.T, url string) (string, error) {
	t.Helper()
	res, err := Client(5 * time.Second).Get(url)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	return string(b), err
}

// The address the listener gave carries their requests: the proxy sees the
// whole URL, not just its path, and answers in its place.
func TestCustomProxyCarriesTheRequests(t *testing.T) {
	defer Set(System, "")
	srv, seen := carrier(t)
	if err := Set(Custom, srv.URL); err != nil {
		t.Fatal(err)
	}
	if body, err := get(t, "http://music.example/cover.jpg"); err != nil || body != "carried" {
		t.Fatalf("got %q, %v", body, err)
	}
	if got := seen(); len(got) != 1 || got[0] != "http://music.example/cover.jpg" {
		t.Errorf("the proxy carried %q", got)
	}
}

// Going direct, no proxy carries anything: the request reaches its host or
// fails on its own.
func TestDirectKeepsTheProxyOut(t *testing.T) {
	defer Set(System, "")
	srv, seen := carrier(t)
	if err := Set(Custom, srv.URL); err != nil {
		t.Fatal(err)
	}
	if err := Set(Direct, ""); err != nil {
		t.Fatal(err)
	}
	// Nothing listens there, so only the proxy could have answered.
	if body, err := get(t, "http://127.0.0.1:1/song.mp3"); err == nil {
		t.Errorf("direct reached %q", body)
	}
	if got := seen(); len(got) != 0 {
		t.Errorf("the proxy carried %q", got)
	}
}

// Following the system, a host on this machine is reached directly: no proxy
// of the system's, of the listener's, stands between the app and a plugin's
// program there.
func TestSystemReachesLoopbackDirectly(t *testing.T) {
	defer Set(System, "")
	srv, seen := carrier(t)
	if err := Set(System, ""); err != nil {
		t.Fatal(err)
	}
	if body, err := get(t, srv.URL+"/local"); err != nil || body != "carried" {
		t.Fatalf("got %q, %v", body, err)
	}
	// Carried by the server itself, which reads a path; through a proxy it
	// would have read the whole URL.
	if got := seen(); len(got) != 1 || got[0] != "/local" {
		t.Errorf("the server was asked for %q", got)
	}
}

// An address no proxy goes by leaves the way out as it was, and is refused.
func TestSetRefusesAnAddressNoProxyGoesBy(t *testing.T) {
	defer Set(System, "")
	srv, seen := carrier(t)
	if err := Set(Custom, srv.URL); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "   ", "ftp://127.0.0.1:21", "http://", "://localhost:8080"} {
		if err := Set(Custom, bad); err == nil {
			t.Errorf("took %q for an address", bad)
		}
	}
	if body, err := get(t, "http://music.example/album"); err != nil || body != "carried" {
		t.Fatalf("got %q, %v", body, err)
	}
	if got := seen(); len(got) != 1 {
		t.Errorf("the proxy of the way out before carried %q", got)
	}
}

// An address reads as the listener may write it, and is kept as transport
// understands it.
func TestParseURL(t *testing.T) {
	ok := map[string]string{
		"127.0.0.1:7890":           "http://127.0.0.1:7890",
		"  127.0.0.1:7890  ":       "http://127.0.0.1:7890",
		"http://127.0.0.1:7890":    "http://127.0.0.1:7890",
		"https://proxy.example":    "https://proxy.example",
		"socks5://127.0.0.1:1080":  "socks5://127.0.0.1:1080",
		"SOCKS5H://127.0.0.1:1080": "socks5://127.0.0.1:1080",
		"user:pass@127.0.0.1:3128": "http://user:pass@127.0.0.1:3128",
	}
	for in, want := range ok {
		u, err := ParseURL(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if got := u.String(); got != want {
			t.Errorf("%q reads as %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", "ftp://127.0.0.1:21", "http://", "localhost:port"} {
		if u, err := ParseURL(bad); err == nil {
			t.Errorf("%q reads as %q", bad, u)
		}
	}
}
