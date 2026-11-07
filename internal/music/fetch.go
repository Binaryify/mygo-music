package music

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Agent is who the app says it is when fetching what it plays and shows.
const Agent = "MyGoMusic/1.0"

// fetchClient gets covers: they're small, and a slow one shouldn't hold a
// slot for long.
var fetchClient = &http.Client{Timeout: 30 * time.Second}

// maxFetch is the most a cover may take.
const maxFetch = 64 << 20

// CoverHeaders, when set, answers the headers to ask for a picture at u with,
// such as the cookie of a source that keeps its covers behind its session.
// After the server turned those down, it's asked again with stale, to forget
// what it answered and find them anew. It may answer nil.
var CoverHeaders func(u *url.URL, stale bool) map[string]string

// Fetch returns what's at url, of the web or a file://, such as a cover.
func Fetch(u string) ([]byte, error) {
	if path, ok := PathOf(u); ok {
		return os.ReadFile(path)
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return nil, err
	}
	var headers map[string]string
	if CoverHeaders != nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		headers = CoverHeaders(parsed, false)
	}
	res, err := fetchWith(u, headers)
	if err == nil && headers != nil && (res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden) {
		// What it asked with may have run out, such as a session's cookie.
		if again := CoverHeaders(parsed, true); again != nil {
			res.Body.Close()
			res, err = fetchWith(u, again)
		}
	}
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New(res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, maxFetch))
}

// fetchWith asks for u with headers, which go no further than its host: a
// redirect elsewhere is asked without them.
func fetchWith(u string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", Agent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := HeadersStay(fetchClient, req.URL.Hostname(), headers)
	return client.Do(req)
}

// HeadersStay returns client, or, with headers to send, one like it that
// asks a redirect to a host other than host, or one below it, without them,
// so that what a source asks with, such as its token, reaches no one else.
// It's the rule Go keeps for Cookie and Authorization, for every header.
func HeadersStay(client *http.Client, host string, headers map[string]string) *http.Client {
	if len(headers) == 0 {
		return client
	}
	host = strings.ToLower(host)
	c := *client
	c.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if to := strings.ToLower(next.URL.Hostname()); to != host && !strings.HasSuffix(to, "."+host) {
			for k := range headers {
				next.Header.Del(k)
			}
		}
		return nil
	}
	return &c
}

// FileURL is the file:// URL of a path.
func FileURL(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path // C:/Music
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

// PathOf returns the path a file:// URL names, and whether u is one.
func PathOf(u string) (string, bool) {
	if !strings.HasPrefix(u, "file:") {
		return "", false
	}
	parsed, err := url.Parse(u)
	if err != nil || parsed.Path == "" {
		return "", false
	}
	path := parsed.Path
	if runtime.GOOS == "windows" {
		path = strings.TrimPrefix(path, "/")
	}
	return filepath.FromSlash(path), true
}
