package clevelandart

import (
	"context"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes the Cleveland Museum of Art as a kit Domain: a driver
// that a multi-domain host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/clevelandart-cli/clevelandart"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then
// dereferences clevelandart:// URIs by routing to the operations Register
// installs. The same Domain also builds the standalone clevelandart binary
// (see cli.NewApp), so the binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the Cleveland Museum of Art driver. It carries no state; the
// per-run client is built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "clevelandart",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "clevelandart",
			Short:  "Explore the Cleveland Museum of Art open access collection.",
			Long: `clevelandart lets you search and explore the Cleveland Museum of Art
open access collection over plain HTTPS. No API key required.

Search artworks, look up individual pieces by accession number, and find
creators. Output is clean JSON that pipes into the rest of your tools.`,
			Site: Host,
			Repo: "https://github.com/tamnd/clevelandart-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{
		Name:    "search",
		Group:   "read",
		Summary: "Search artworks by keyword",
		Args:    []kit.Arg{{Name: "query", Help: "search query"}},
	}, searchArtworks)

	kit.Handle(app, kit.OpMeta{
		Name:     "artwork",
		Group:    "read",
		Single:   true,
		Summary:  "Fetch an artwork by accession number or numeric ID",
		URIType:  "artwork",
		Resolver: true,
		Args:     []kit.Arg{{Name: "id", Help: "accession number (1926.197) or numeric ID"}},
	}, getArtwork)

	kit.Handle(app, kit.OpMeta{
		Name:    "creators",
		Group:   "read",
		Summary: "Search creators/artists by keyword",
		Args:    []kit.Arg{{Name: "query", Help: "creator/artist search query"}},
	}, searchCreators)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type searchInput struct {
	Query    string  `kit:"arg" help:"search query"`
	Type     string  `kit:"flag" help:"artwork type filter (painting, photograph, drawing, print, etc.)"`
	HasImage bool    `kit:"flag" help:"only artworks with images"`
	Limit    int     `kit:"flag,inherit" help:"max results"`
	Client   *Client `kit:"inject"`
}

type artworkInput struct {
	ID     string  `kit:"arg" help:"accession number (1926.197) or numeric ID"`
	Client *Client `kit:"inject"`
}

type creatorsInput struct {
	Query  string  `kit:"arg" help:"creator/artist search query"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func searchArtworks(ctx context.Context, in searchInput, emit func(Artwork) error) error {
	artworks, err := in.Client.SearchArtworks(ctx, in.Query, in.Type, in.HasImage, in.Limit)
	if err != nil {
		return err
	}
	for _, a := range artworks {
		if err := emit(a); err != nil {
			return err
		}
	}
	return nil
}

func getArtwork(ctx context.Context, in artworkInput, emit func(*Artwork) error) error {
	a, err := in.Client.GetArtwork(ctx, in.ID)
	if err != nil {
		return err
	}
	return emit(a)
}

func searchCreators(ctx context.Context, in creatorsInput, emit func(Creator) error) error {
	creators, err := in.Client.SearchCreators(ctx, in.Query, in.Limit)
	if err != nil {
		return err
	}
	for _, cr := range creators {
		if err := emit(cr); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: the URI-native string functions, pure and network-free ---

// Classify turns any accepted input into the canonical (type, id), so `ant
// resolve` and `ant url` touch no network.
func (Domain) Classify(input string) (uriType, id string, err error) {
	t, v := Classify(input)
	switch t {
	case "accession", "id":
		return "artwork", v, nil
	default:
		return "", "", errs.Usage("unrecognized clevelandart reference: %q", input)
	}
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "artwork":
		return Locate(uriType, id), nil
	default:
		return "", errs.Usage("clevelandart has no resource type %q", uriType)
	}
}

// mapErr converts a library error into the kit error kind that carries the
// right exit code.
func mapErr(err error) error {
	return err
}
