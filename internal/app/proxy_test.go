package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/netclient"
)

// settingsOf renders the settings of page a, as the window shows them.
func settingsOf(t *testing.T, a *app, page string, width, height int) *ui.Tester {
	t.Helper()
	a.settingsPage = page
	tt := ui.NewTester(a.settingsView, width, height)
	tt.Frame()
	return tt
}

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

// carried fetches url, as the app would, and answers what came back.
func carried(t *testing.T) (string, error) {
	t.Helper()
	res, err := netclient.Client(5 * time.Second).Get("http://music.example/cover.jpg")
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	return string(b), err
}

// chooseProxy picks want in the row of the proxy, which shows current.
func chooseProxy(t *testing.T, tt *ui.Tester, current, want string) {
	t.Helper()
	at, ok := tt.Find(current)
	if !ok {
		t.Fatalf("no proxy showing %q", current)
	}
	tt.ClickAt(at.X+at.W/2, at.Y+at.H/2)
	tt.Frame()
	if err := tt.Click(want); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
}

// A choice of proxy keeps itself, and the app goes that way from then on:
// direct, no proxy the listener set the system to carries its requests.
func TestProxyChoiceApplies(t *testing.T) {
	defer netclient.Set(netclient.System, "")
	a := newTestApp()
	srv, seen := carrier(t)
	a.settings.Proxy, a.settings.ProxyAddress = netclient.Custom, srv.URL
	a.applyProxy()
	tt := settingsOf(t, a, "network", 760, 900)
	chooseProxy(t, tt, "Custom", "Direct")
	if a.settings.Proxy != netclient.Direct {
		t.Fatalf("the settings keep %q, want direct", a.settings.Proxy)
	}
	res, err := netclient.Client(5 * time.Second).Get("http://127.0.0.1:1/song.mp3")
	if err == nil {
		res.Body.Close()
		t.Error("direct reached a host nothing answers at")
	}
	if got := seen(); len(got) != 0 {
		t.Errorf("the proxy carried %q after going direct", got)
	}
}

// The address typed in the settings is the proxy the app's requests go
// through, once submitted, and until another choice is made.
func TestProxyAddressCarriesTheRequests(t *testing.T) {
	defer netclient.Set(netclient.System, "")
	a := newTestApp()
	srv, seen := carrier(t)
	tt := settingsOf(t, a, "network", 760, 900)
	chooseProxy(t, tt, "Follow the System", "Custom")
	if a.settings.Proxy != netclient.Custom {
		t.Fatalf("the settings keep %q, want custom", a.settings.Proxy)
	}
	// The address is typed into the row that shows once a proxy of the
	// listener's own is chosen, its field at the end of the row, a line under
	// its title.
	at, ok := tt.Find("Proxy Address")
	if !ok {
		t.Fatal("no proxy address row")
	}
	tt.ClickAt(620, at.Y+at.H/2+20)
	tt.Type(srv.URL)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.settings.ProxyAddress != srv.URL {
		t.Fatalf("the settings keep the address %q, want %q", a.settings.ProxyAddress, srv.URL)
	}
	if body, err := carried(t); err != nil || body != "carried" {
		t.Fatalf("got %q, %v", body, err)
	}
	if got := seen(); len(got) != 1 || got[0] != "http://music.example/cover.jpg" {
		t.Errorf("the proxy carried %q", got)
	}
}
