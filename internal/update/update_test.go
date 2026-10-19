package update

import "testing"

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, cur string
		want        bool
	}{
		{"v1.0.13", "1.0.12", true},
		{"v1.0.12", "1.0.12", false},
		{"v1.0.9", "1.0.12", false},
		{"v1.1.0", "1.0.99", true},
		{"v2.0", "1.9.9", true},
		{"v1.0.12", "dev", true},
		{"garbage", "1.0.0", false},
		{"v1.0.12-beta", "1.0.11", true},
	}
	for _, c := range cases {
		if got := Newer(c.latest, c.cur); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, attendu %v", c.latest, c.cur, got, c.want)
		}
	}
}

func TestParseRelease(t *testing.T) {
	js := `{"tag_name":"v1.0.7","html_url":"https://github.com/o/r/releases/tag/v1.0.7","assets":[
	{"name":"Lumen-x64.msi","browser_download_url":"https://x/x64","size":10},
	{"name":"Lumen-arm64.msi","browser_download_url":"https://x/arm","size":12}]}`
	r, err := ParseRelease([]byte(js))
	if err != nil || r.Tag != "v1.0.7" || len(r.Assets) != 2 {
		t.Fatalf("lecture: %+v %v", r, err)
	}
	if a, ok := r.AssetFor(ArchName("arm64")); !ok || a.URL != "https://x/arm" {
		t.Errorf("arm64: %+v %v", a, ok)
	}
	if _, ok := r.AssetFor(ArchName("386")); ok {
		t.Error("pas de x86 dans cette release")
	}
	if a, ok := r.AssetFor(ArchName("amd64")); !ok || a.Size != 10 {
		t.Errorf("x64: %+v %v", a, ok)
	}
}
