package plugin

import (
	"testing"
	"time"
)

// A plugin answers the headers of a cover, from its settings as they are,
// and how long they hold; one answering nothing has none.
func TestImageHeaders(t *testing.T) {
	p := load(t, `{"id":"x","permissions":{"network":["music.local"]}}`, `
export function imageHeaders({ url }) {
  if (!url.startsWith("https://music.local/")) return null;
  return { headers: { cookie: "music-token=" + host.setting("token") }, maxAge: 60 };
}`)
	if err := p.SetSetting("token", "abc"); err != nil {
		t.Fatal(err)
	}
	headers, age, err := p.ImageHeaders("https://music.local/cover?id=1")
	if err != nil {
		t.Fatal(err)
	}
	if headers["cookie"] != "music-token=abc" || age != time.Minute {
		t.Errorf("answered %v for %v", headers, age)
	}
	if headers, _, err := p.ImageHeaders("https://elsewhere.example/c.jpg"); err != nil || headers != nil {
		t.Errorf("answered %v, %v for a cover not its own", headers, err)
	}
}

// A plugin reaches a host its network permission names, or a domain above
// it names, before one allowed any host.
func TestReaches(t *testing.T) {
	p := load(t, `{"id":"x","permissions":{"network":["api.example.com","*.cdn.example","*"]}}`, `export function home() { return {}; }`)
	for host, want := range map[string][2]bool{
		"API.example.com": {true, true},
		"img.cdn.example": {true, true},
		"cdn.example":     {true, false},
		"other.example":   {true, false},
	} {
		if reaches, named := p.Reaches(host); reaches != want[0] || named != want[1] {
			t.Errorf("%s: reaches %v, named %v", host, reaches, named)
		}
	}
	none := load(t, `{"id":"y"}`, `export function home() { return {}; }`)
	if reaches, _ := none.Reaches("api.example.com"); reaches {
		t.Error("reached a host without the network permission")
	}
}
