package clevelandart

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestSearchArtworks(t *testing.T) {
	resp := wireListResponse{
		Data: []wireArtwork{
			{
				ID:        1,
				Accession: "1926.197",
				Title:     "Water Lilies",
				Date:      "1906",
				Type:      "Painting",
				Technique: "Oil on canvas",
				Creators:  []wireCreator{{Description: "Claude Monet", Role: "artist"}},
				Images:    wireImages{Web: &wireImage{URL: "https://example.com/img.jpg"}},
			},
		},
	}
	b, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	artworks, err := c.searchArtworksURL(context.Background(), srv.URL+"/artworks/")
	if err != nil {
		t.Fatal(err)
	}
	if len(artworks) != 1 {
		t.Fatalf("len(artworks) = %d, want 1", len(artworks))
	}
	if artworks[0].Title != "Water Lilies" {
		t.Errorf("Title = %q, want %q", artworks[0].Title, "Water Lilies")
	}
	if artworks[0].Artist != "Claude Monet" {
		t.Errorf("Artist = %q, want %q", artworks[0].Artist, "Claude Monet")
	}
	if artworks[0].ImageURL != "https://example.com/img.jpg" {
		t.Errorf("ImageURL = %q", artworks[0].ImageURL)
	}
}

func TestGetArtworkByAccession(t *testing.T) {
	resp := wireSingleResponse{
		Data: wireArtwork{
			ID:        42,
			Accession: "1926.197",
			Title:     "Starry Night",
			Date:      "1889",
			Type:      "Painting",
			Technique: "Oil on canvas",
			Creators:  []wireCreator{{Description: "Vincent van Gogh"}},
		},
	}
	b, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	a, err := c.getArtworkURL(context.Background(), srv.URL+"/artworks/1926.197")
	if err != nil {
		t.Fatal(err)
	}
	if a.Title != "Starry Night" {
		t.Errorf("Title = %q, want Starry Night", a.Title)
	}
	if a.Artist != "Vincent van Gogh" {
		t.Errorf("Artist = %q, want Vincent van Gogh", a.Artist)
	}
}

func TestSearchCreators(t *testing.T) {
	resp := wireCreatorList{
		Data: []wireCreatorEntry{
			{ID: 10, Description: "Edgar Degas"},
			{ID: 11, Description: "Claude Monet"},
		},
	}
	b, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	creators, err := c.searchCreatorsURL(context.Background(), srv.URL+"/creators/")
	if err != nil {
		t.Fatal(err)
	}
	if len(creators) != 2 {
		t.Fatalf("len(creators) = %d, want 2", len(creators))
	}
	if creators[0].Description != "Edgar Degas" {
		t.Errorf("creators[0].Description = %q, want Edgar Degas", creators[0].Description)
	}
}

func TestClassifyFunc(t *testing.T) {
	cases := []struct {
		in      string
		wantTyp string
		wantID  string
	}{
		{"1926.197", "accession", "1926.197"},
		{"436532", "id", "436532"},
		{"monet", "query", "monet"},
		{"impressionism", "query", "impressionism"},
	}
	for _, tc := range cases {
		typ, id := Classify(tc.in)
		if typ != tc.wantTyp || id != tc.wantID {
			t.Errorf("Classify(%q) = (%q, %q), want (%q, %q)", tc.in, typ, id, tc.wantTyp, tc.wantID)
		}
	}
}

func TestLocateFunc(t *testing.T) {
	got := Locate("accession", "1926.197")
	want := SiteBase + "/1926.197"
	if got != want {
		t.Errorf("Locate = %q, want %q", got, want)
	}
}
