package music

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// useCoverHeaders sets CoverHeaders for a test, putting back what was there.
func useCoverHeaders(t *testing.T, f func(*url.URL, bool) map[string]string) {
	was := CoverHeaders
	CoverHeaders = f
	t.Cleanup(func() { CoverHeaders = was })
}

// A cover is asked for with the headers its source gives, and turned down,
// with those found anew, as after the session's cookie ran out.
func TestFetchWithCoverHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "token=fresh" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte("cover"))
	}))
	defer srv.Close()
	var stale []bool
	useCoverHeaders(t, func(u *url.URL, again bool) map[string]string {
		stale = append(stale, again)
		if again {
			return map[string]string{"Cookie": "token=fresh"}
		}
		return map[string]string{"Cookie": "token=expired"}
	})
	b, err := Fetch(srv.URL + "/cover?id=1")
	if err != nil || string(b) != "cover" {
		t.Fatalf("got %q, %v", b, err)
	}
	if len(stale) != 2 || stale[0] || !stale[1] {
		t.Errorf("asked for headers %v", stale)
	}
}

// A cover's headers go no further than its host: a redirect to another is
// asked for without them, so a source's cookie never reaches a stranger.
func TestCoverHeadersStayOnTheirHost(t *testing.T) {
	var leaked string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("X-Token")
		w.Write([]byte("cover"))
	}))
	defer other.Close()
	// 127.0.0.1 and localhost are other hosts, as far as the headers go.
	elsewhere := "http://localhost:" + must(url.Parse(other.URL)).Port() + "/c.jpg"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Token") != "secret" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		http.Redirect(w, r, elsewhere, http.StatusFound)
	}))
	defer srv.Close()
	useCoverHeaders(t, func(u *url.URL, _ bool) map[string]string {
		if u.Hostname() != "127.0.0.1" {
			return nil
		}
		return map[string]string{"X-Token": "secret"}
	})
	if _, err := Fetch(srv.URL + "/c.jpg"); err != nil {
		t.Fatal(err)
	}
	if leaked != "" {
		t.Errorf("the redirect took the header along: %q", leaked)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
