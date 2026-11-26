// Package recipes provides the JC catalog and no-execution planning/export surfaces.
package recipes

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/justyn-clark/loopexec/internal/workflow"
)

//go:embed catalog.json
var catalogBytes []byte

type Catalog struct {
	CatalogSHA256          string   `json:"catalog_sha256,omitempty"`
	SchemaVersion          int      `json:"schema_version"`
	Brand                  string   `json:"brand"`
	Collection             string   `json:"collection"`
	Status                 string   `json:"status"`
	Origin                 string   `json:"origin"`
	SourceIntro            string   `json:"source_intro"`
	SourceSocialIntro      string   `json:"source_social_intro"`
	SourceSocialPriorities []string `json:"source_social_priorities"`
	SourceClosing          string   `json:"source_closing"`
	Recipes                []Recipe `json:"recipes"`
}
type Input struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Description string          `json:"description"`
	Required    bool            `json:"required"`
	Default     json.RawMessage `json:"default,omitempty"`
	Minimum     *float64        `json:"minimum,omitempty"`
	Maximum     *float64        `json:"maximum,omitempty"`
}
type Recipe struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Category       string   `json:"category"`
	Summary        string   `json:"summary"`
	SourceIdea     string   `json:"source_idea"`
	Inputs         []Input  `json:"inputs"`
	Attempt        string   `json:"attempt"`
	Checks         []string `json:"checks"`
	Protect        []string `json:"protect"`
	Evidence       []string `json:"evidence"`
	Demo           string   `json:"demo"`
	Tags           []string `json:"tags"`
	ExpectedHalt   string   `json:"expected_halt"`
	SocialPriority int      `json:"social_priority"`
	AdapterStatus  string   `json:"adapter_status"`
	SourceIndex    int      `json:"source_index"`
}
type Entry struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Category       string `json:"category"`
	AdapterStatus  string `json:"adapter_status"`
	SocialPriority int    `json:"social_priority,omitempty"`
}
type Suggestion struct {
	Entry   Entry    `json:"recipe"`
	Score   int      `json:"score"`
	Matched []string `json:"matched"`
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9-]{0,59}$`)

func Load() (Catalog, error) {
	var c Catalog
	if err := workflow.Decode(catalogBytes, &c); err != nil {
		return c, err
	}
	if c.SchemaVersion != 1 || c.Brand != "JC" {
		return c, fmt.Errorf("unsupported recipe catalog")
	}
	seen := map[string]bool{}
	for i, r := range c.Recipes {
		if !identifier.MatchString(r.ID) || seen[r.ID] || r.SourceIndex != i+1 || r.SourceIdea == "" || len(r.Checks) == 0 || len(r.Inputs) == 0 {
			return c, fmt.Errorf("invalid recipe at index %d", i)
		}
		seen[r.ID] = true
		names := map[string]bool{}
		for _, input := range r.Inputs {
			if !identifier.MatchString(strings.ReplaceAll(input.Name, "_", "-")) || names[input.Name] {
				return c, fmt.Errorf("invalid input in %s", r.ID)
			}
			names[input.Name] = true
			switch input.Type {
			case "path", "string", "strings", "number", "integer":
			default:
				return c, fmt.Errorf("unsupported input type in %s", r.ID)
			}
			if input.Default != nil {
				if _, err := validateInput(input, input.Default); err != nil {
					return c, err
				}
			}
		}
	}
	return c, nil
}
func Digest() string { h := sha256.Sum256(catalogBytes); return hex.EncodeToString(h[:]) }
func (c Catalog) Find(id string) (Recipe, error) {
	for _, r := range c.Recipes {
		if r.ID == id {
			return r, nil
		}
	}
	return Recipe{}, fmt.Errorf("unknown recipe %q; use recipes list", id)
}
func entry(r Recipe) Entry {
	return Entry{r.ID, r.Title, r.Category, r.AdapterStatus, r.SocialPriority}
}
func (c Catalog) List(category string) []Entry {
	out := []Entry{}
	for _, r := range c.Recipes {
		if category == "" || r.Category == category {
			out = append(out, entry(r))
		}
	}
	return out
}
func tokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}
func (c Catalog) Suggest(query string, limit int) []Suggestion {
	aliases := map[string]string{"louder": "loudness", "cheating": "gaming", "cheat": "gaming", "normalize": "loudness", "a11y": "accessibility", "typescript": "types", "cycle": "oscillation", "compress": "compression"}
	stop := map[string]bool{"a": true, "an": true, "the": true, "my": true, "this": true, "make": true, "i": true, "want": true, "to": true, "and": true, "of": true, "for": true, "in": true, "with": true, "fix": true}
	terms := map[string]bool{}
	for _, t := range tokens(query) {
		if alias := aliases[t]; alias != "" {
			t = alias
		}
		if !stop[t] {
			terms[t] = true
		}
	}
	out := []Suggestion{}
	for _, r := range c.Recipes {
		fields := []string{r.ID + " " + r.Title + " " + strings.Join(r.Tags, " "), r.Category, r.Summary}
		score := 0
		matched := []string{}
		for term := range terms {
			found := false
			for i, f := range fields {
				for _, t := range tokens(f) {
					if t == term {
						score += 3 - i
						found = true
						break
					}
				}
			}
			if found {
				matched = append(matched, term)
			}
		}
		if score > 0 {
			sort.Strings(matched)
			out = append(out, Suggestion{entry(r), score, matched})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Entry.ID < out[j].Entry.ID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
