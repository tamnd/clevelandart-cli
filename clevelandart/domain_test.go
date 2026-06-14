package clevelandart

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring, which need no network. HTTP behaviour is in clevelandart_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "clevelandart" {
		t.Errorf("Scheme = %q, want clevelandart", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "clevelandart" {
		t.Errorf("Identity.Binary = %q, want clevelandart", info.Identity.Binary)
	}
}

func TestDomainClassify(t *testing.T) {
	cases := []struct {
		in      string
		wantTyp string
		wantID  string
		wantErr bool
	}{
		{"1926.197", "artwork", "1926.197", false},
		{"436532", "artwork", "436532", false},
		{"monet", "", "", true}, // query strings are not classifiable as URI types
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Classify(%q): want error, got (%q, %q, nil)", tc.in, typ, id)
			}
			continue
		}
		if err != nil || typ != tc.wantTyp || id != tc.wantID {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.wantTyp, tc.wantID)
		}
	}
}

func TestDomainLocate(t *testing.T) {
	got, err := Domain{}.Locate("artwork", "1926.197")
	want := SiteBase + "/1926.197"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}

	_, err = Domain{}.Locate("unknown", "foo")
	if err == nil {
		t.Error("Locate unknown type: want error, got nil")
	}
}

func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	a := &Artwork{
		ID:        42,
		Accession: "1926.197",
		Title:     "Water Lilies",
		URL:       SiteBase + "/1926.197",
	}
	u, err := h.Mint(a)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if want := "clevelandart://artwork/42"; u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("clevelandart", "1926.197")
	if err != nil {
		t.Fatalf("ResolveOn: %v", err)
	}
	if got.String() != "clevelandart://artwork/1926.197" {
		t.Errorf("ResolveOn = %q, want clevelandart://artwork/1926.197", got.String())
	}
}
