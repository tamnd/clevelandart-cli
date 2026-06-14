// Package clevelandart is the library behind the clevelandart command line:
// the HTTP client, request shaping, and the typed data models for the
// Cleveland Museum of Art open access API.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
package clevelandart

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultUserAgent identifies the client to the Cleveland Museum of Art API.
const DefaultUserAgent = "clevelandart-cli/dev (+https://github.com/tamnd/clevelandart-cli)"

// APIBase is the root every API request is built from.
const APIBase = "https://openaccess-api.clevelandart.org/api"

// SiteBase is the public site root, used for constructing human-facing URLs.
const SiteBase = "https://clevelandart.org/art"

// Host is the site this client is associated with.
const Host = "clevelandart.org"

// Config holds the tunable client settings.
type Config struct {
	Rate    time.Duration
	Retries int
	Timeout time.Duration
}

// DefaultConfig returns sensible defaults: 200ms pacing, 3 retries, 15s timeout.
func DefaultConfig() Config {
	return Config{
		Rate:    200 * time.Millisecond,
		Retries: 3,
		Timeout: 15 * time.Second,
	}
}

// Client talks to the Cleveland Museum of Art open access API.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	Rate      time.Duration
	Retries   int

	last time.Time
}

// NewClient returns a Client with the defaults from DefaultConfig.
func NewClient() *Client {
	cfg := DefaultConfig()
	return &Client{
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		UserAgent: DefaultUserAgent,
		Rate:      cfg.Rate,
		Retries:   cfg.Retries,
	}
}

// Get fetches url and returns the response body. It paces and retries
// according to the client's settings.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- Output types ---

// Artwork is a single artwork record from the Cleveland Museum of Art.
type Artwork struct {
	ID         int    `json:"id" kit:"id"`
	Accession  string `json:"accession_number"`
	Title      string `json:"title"`
	Date       string `json:"date"`
	Type       string `json:"type"`
	Medium     string `json:"medium"`
	Artist     string `json:"artist"`
	Department string `json:"department"`
	URL        string `json:"url"`
	ImageURL   string `json:"image_url"`
}

// Creator is an artist or creator record.
type Creator struct {
	ID          int    `json:"id" kit:"id"`
	Description string `json:"description"`
}

// --- Wire types ---

type wireListResponse struct {
	Data []wireArtwork `json:"data"`
	Info wireInfo      `json:"info"`
}

type wireInfo struct {
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

type wireArtwork struct {
	ID         int            `json:"id"`
	Accession  string         `json:"accession_number"`
	Title      string         `json:"title"`
	Date       string         `json:"creation_date"`
	Type       string         `json:"type"`
	Technique  string         `json:"technique"`
	Department string         `json:"department"`
	URL        string         `json:"url"`
	Creators   []wireCreator  `json:"creators"`
	Images     wireImages     `json:"images"`
}

type wireCreator struct {
	Description string `json:"description"`
	Role        string `json:"role"`
}

type wireImages struct {
	Web *wireImage `json:"web"`
}

type wireImage struct {
	URL string `json:"url"`
}

type wireSingleResponse struct {
	Data wireArtwork `json:"data"`
}

type wireCreatorList struct {
	Data []wireCreatorEntry `json:"data"`
}

type wireCreatorEntry struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
}

// mapArtwork converts a wireArtwork into an Artwork output record.
func mapArtwork(w wireArtwork) Artwork {
	artist := ""
	if len(w.Creators) > 0 {
		artist = w.Creators[0].Description
	}
	imageURL := ""
	if w.Images.Web != nil {
		imageURL = w.Images.Web.URL
	}
	return Artwork{
		ID:         w.ID,
		Accession:  w.Accession,
		Title:      w.Title,
		Date:       w.Date,
		Type:       w.Type,
		Medium:     w.Technique,
		Artist:     artist,
		Department: w.Department,
		URL:        w.URL,
		ImageURL:   imageURL,
	}
}

// --- Client methods ---

// SearchArtworks searches for artworks by keyword with optional filters.
// GET /artworks/?q={query}&type={type}&has_image={1|}&limit={n}
func (c *Client) SearchArtworks(ctx context.Context, query, artType string, hasImage bool, limit int) ([]Artwork, error) {
	params := url.Values{}
	if query != "" {
		params.Set("q", query)
	}
	if artType != "" {
		params.Set("type", artType)
	}
	if hasImage {
		params.Set("has_image", "1")
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	apiURL := APIBase + "/artworks/?" + params.Encode()
	return c.searchArtworksURL(ctx, apiURL)
}

func (c *Client) searchArtworksURL(ctx context.Context, apiURL string) ([]Artwork, error) {
	body, err := c.Get(ctx, apiURL)
	if err != nil {
		return nil, err
	}

	var resp wireListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse artworks response: %w", err)
	}

	out := make([]Artwork, len(resp.Data))
	for i, w := range resp.Data {
		out[i] = mapArtwork(w)
	}
	return out, nil
}

// GetArtwork fetches a single artwork by accession number or numeric ID.
// GET /artworks/{id}
func (c *Client) GetArtwork(ctx context.Context, id string) (*Artwork, error) {
	apiURL := APIBase + "/artworks/" + strings.TrimSpace(id)
	return c.getArtworkURL(ctx, apiURL)
}

func (c *Client) getArtworkURL(ctx context.Context, apiURL string) (*Artwork, error) {
	body, err := c.Get(ctx, apiURL)
	if err != nil {
		return nil, err
	}

	var resp wireSingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse artwork response: %w", err)
	}

	a := mapArtwork(resp.Data)
	return &a, nil
}

// SearchCreators searches for creators/artists by keyword.
// GET /creators/?q={query}&limit={n}
func (c *Client) SearchCreators(ctx context.Context, query string, limit int) ([]Creator, error) {
	params := url.Values{}
	if query != "" {
		params.Set("q", query)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	apiURL := APIBase + "/creators/?" + params.Encode()
	return c.searchCreatorsURL(ctx, apiURL)
}

func (c *Client) searchCreatorsURL(ctx context.Context, apiURL string) ([]Creator, error) {
	body, err := c.Get(ctx, apiURL)
	if err != nil {
		return nil, err
	}

	var resp wireCreatorList
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse creators response: %w", err)
	}

	out := make([]Creator, len(resp.Data))
	for i, e := range resp.Data {
		out[i] = Creator{ID: e.ID, Description: e.Description}
	}
	return out, nil
}

// Classify determines the type and canonical id of an input string.
// - "1926.197" (contains ".") → ("accession", "1926.197")
// - "436532" (numeric)        → ("id", "436532")
// - "monet"                   → ("query", "monet")
func Classify(input string) (uriType, id string) {
	input = strings.TrimSpace(input)
	if strings.Contains(input, ".") {
		return "accession", input
	}
	if _, err := strconv.Atoi(input); err == nil {
		return "id", input
	}
	return "query", input
}

// Locate returns the public site URL for a (type, id) pair.
func Locate(uriType, id string) string {
	return SiteBase + "/" + id
}
