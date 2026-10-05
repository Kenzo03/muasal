// Package ask answers questions from the tickets a user may open (FSD §10,
// §11): it detects the scope, retrieves evidence under the visibility
// predicate, packs it, prompts the model for schema-constrained claims and
// drops every claim without a valid citation.
package ask

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/kenzo03/zettra/server/internal/db"
)

// Asker is who asks: grants come from the database in each query (§11.1).
type Asker struct {
	UserID  int64
	IsAdmin bool
	Locale  string         // UI language, "id" or "en"
	TZ      *time.Location // date phrases and date chips use it (§10.2)
}

// Catalog is what detection may name: only clients, nodes and people the
// asker may see, so detection reveals nothing (§11.2).
type Catalog struct {
	Clients []db.ListScopeClientsRow
	Nodes   []db.ListScopeNodesRow
	People  []db.ListScopePeopleRow
}

// LoadCatalog reads the asker's catalog.
func LoadCatalog(ctx context.Context, q *db.Queries, a Asker) (Catalog, error) {
	var c Catalog
	var err error
	if c.Clients, err = q.ListScopeClients(ctx, db.ListScopeClientsParams{IsAdmin: a.IsAdmin, UserID: a.UserID}); err != nil {
		return c, err
	}
	if c.Nodes, err = q.ListScopeNodes(ctx, db.ListScopeNodesParams{IsAdmin: a.IsAdmin, UserID: a.UserID}); err != nil {
		return c, err
	}
	c.People, err = q.ListScopePeople(ctx, db.ListScopePeopleParams{IsAdmin: a.IsAdmin, UserID: a.UserID})
	return c, err
}

// Detected is what the question itself names (§11.2). Dates are whole days in
// the asker's timezone; To is inclusive.
type Detected struct {
	ClientIDs  []int64    `json:"client_ids,omitempty"`
	NodeIDs    []int64    `json:"node_ids,omitempty"`
	UserIDs    []int64    `json:"user_ids,omitempty"`
	ContactIDs []int64    `json:"contact_ids,omitempty"`
	From       *time.Time `json:"from,omitempty"`
	To         *time.Time `json:"to,omitempty"`
	Keys       []string   `json:"keys,omitempty"`
	Labels     []Label    `json:"labels,omitempty"` // names for the chips above, in that order
	// AllClients: the question asks about other or every client too, so a
	// client it names doesn't narrow the scope, nor does an earlier one carry
	// over (MSL-38).
	AllClients bool `json:"all_clients,omitempty"`
}

// Label names one detected chip: a client, a node (by its path) or a person.
type Label struct {
	Kind  string `json:"kind"` // client, node, user or contact
	ID    int64  `json:"id"`
	Label string `json:"label"`
}

var (
	tokenRe = regexp.MustCompile(`\d{4}-\d{2}-\d{2}|[\p{L}\p{N}]+`)
	keyRe   = regexp.MustCompile(`(?i)\b([a-z][a-z0-9]{1,9}-(?:dn)?\d+)\b`)
	// othersRe spots "and for other clients", "semua klien" and the like.
	othersRe = regexp.MustCompile(`(?i)\b(?:(?:other|all|every|each)\s+(?:clients?|customers?)|(?:clients?|customers?)\s+other\s+than|(?:klien|pelanggan)\s+(?:lain(?:nya)?|selain)|(?:semua|seluruh|setiap|tiap|masing-masing)\s+(?:klien|pelanggan))\b`)
)

// Detect finds the clients, nodes, people, dates and ticket keys a question
// names, without a model call (§11.2). now is the asker's current time.
func Detect(cat Catalog, question string, now time.Time) Detected {
	toks := tokenRe.FindAllString(strings.ToLower(question), -1)
	d := Detected{AllClients: othersRe.MatchString(question)}
	for _, c := range cat.Clients {
		if !d.AllClients && matches(toks, append([]string{c.Name, deref(c.Code)}, c.Aliases...), true) {
			d.ClientIDs = append(d.ClientIDs, c.ID)
		}
	}
	d.NodeIDs = deepest(cat.Nodes, toks)
	d.UserIDs, d.ContactIDs = people(cat.People, toks)
	d.From, d.To = dates(toks, now)
	d.Labels = labels(cat, d)
	for _, m := range keyRe.FindAllStringSubmatch(question, -1) {
		if k := strings.ToUpper(m[1]); !slices.Contains(d.Keys, k) {
			d.Keys = append(d.Keys, k)
		}
	}
	return d
}

// Without leaves out the chips the asker removed, and their labels.
func (d Detected) Without(ignore []Label) Detected {
	drop := func(kind string, ids []int64) []int64 {
		return slices.DeleteFunc(slices.Clone(ids), func(id int64) bool {
			return slices.ContainsFunc(ignore, func(l Label) bool { return l.Kind == kind && l.ID == id })
		})
	}
	d.ClientIDs, d.NodeIDs = drop("client", d.ClientIDs), drop("node", d.NodeIDs)
	d.UserIDs, d.ContactIDs = drop("user", d.UserIDs), drop("contact", d.ContactIDs)
	if slices.ContainsFunc(ignore, func(l Label) bool { return l.Kind == "date" }) {
		d.From, d.To = nil, nil
	}
	d.Labels = slices.DeleteFunc(slices.Clone(d.Labels), func(l Label) bool {
		return slices.ContainsFunc(ignore, func(i Label) bool { return i.Kind == l.Kind && i.ID == l.ID })
	})
	for _, f := range []*[]int64{&d.ClientIDs, &d.NodeIDs, &d.UserIDs, &d.ContactIDs} {
		if len(*f) == 0 {
			*f = nil
		}
	}
	if len(d.Labels) == 0 {
		d.Labels = nil
	}
	return d
}

// labels names the detected clients, nodes and people from the catalog, which
// holds only what the asker may see.
func labels(cat Catalog, d Detected) []Label {
	var out []Label
	for _, c := range cat.Clients {
		if slices.Contains(d.ClientIDs, c.ID) {
			out = append(out, Label{Kind: "client", ID: c.ID, Label: c.Name})
		}
	}
	byID := map[int64]db.ListScopeNodesRow{}
	for _, n := range cat.Nodes {
		byID[n.ID] = n
	}
	for _, id := range d.NodeIDs {
		var path []string
		for n, ok := byID[id]; ok; n, ok = byID[deref(n.ParentID)] {
			path = append([]string{n.Name}, path...)
			if n.ParentID == nil {
				break
			}
		}
		out = append(out, Label{Kind: "node", ID: id, Label: strings.Join(path, " › ")})
	}
	for _, p := range cat.People {
		if (p.Kind == "user" && slices.Contains(d.UserIDs, p.ID)) || (p.Kind == "contact" && slices.Contains(d.ContactIDs, p.ID)) {
			out = append(out, Label{Kind: p.Kind, ID: p.ID, Label: p.Name})
		}
	}
	return out
}

// matches reports whether any term appears in the tokens as whole words, case
// aside. With fuzzy, a word of 4+ letters also matches a word with trigram
// similarity of 0.6 or more (§11.2), so "Bumi Logistk" finds "Bumi Logistik".
func matches(toks []string, terms []string, fuzzy bool) bool {
	same := func(a, b string) bool {
		return a == b || (fuzzy && len([]rune(a)) >= 4 && len([]rune(b)) >= 4 && similarity(a, b) >= 0.6)
	}
	for _, term := range terms {
		words := tokenRe.FindAllString(strings.ToLower(term), -1)
		if len(words) == 0 {
			continue
		}
		for i := 0; i+len(words) <= len(toks); i++ {
			if slices.EqualFunc(toks[i:i+len(words)], words, same) {
				return true
			}
		}
	}
	return false
}

// deepest returns the matching nodes, leaving out any node that is an
// ancestor of another match: "the deepest match wins" (§11.2).
func deepest(nodes []db.ListScopeNodesRow, toks []string) []int64 {
	parent := map[int64]*int64{}
	var hit []int64
	for _, n := range nodes {
		parent[n.ID] = n.ParentID
		if matches(toks, append([]string{n.Name, deref(n.Code)}, n.Aliases...), false) {
			hit = append(hit, n.ID)
		}
	}
	var out []int64
	for _, id := range hit {
		ancestor := false
		for _, other := range hit {
			for p := parent[other]; p != nil && !ancestor; p = parent[*p] {
				ancestor = *p == id
			}
		}
		if !ancestor {
			out = append(out, id)
		}
	}
	return out
}

// cueWords come before a person's name: "requested by Budi", "diminta oleh Budi".
var cueWords = []string{"by", "from", "oleh", "dari", "diminta"}

// people matches user and contact names only after a cue word, so names that
// are also common words, such as "Indah", are not taken for people (§11.2).
func people(all []db.ListScopePeopleRow, toks []string) (users, contacts []int64) {
	for i, t := range toks {
		if !slices.Contains(cueWords, t) || i+1 >= len(toks) {
			continue
		}
		for _, p := range all {
			name := tokenRe.FindAllString(strings.ToLower(p.Name), -1)
			full := len(name) > 0 && i+1+len(name) <= len(toks) && slices.Equal(toks[i+1:i+1+len(name)], name)
			first := len(name) > 0 && toks[i+1] == name[0]
			if !full && !first {
				continue
			}
			if p.Kind == "user" && !slices.Contains(users, p.ID) {
				users = append(users, p.ID)
			}
			if p.Kind == "contact" && !slices.Contains(contacts, p.ID) {
				contacts = append(contacts, p.ID)
			}
		}
	}
	return users, contacts
}

// similarity is pg_trgm's: shared trigrams over all trigrams, with each word
// padded by two spaces before and one after.
func similarity(a, b string) float64 {
	ta, tb := trigrams(a), trigrams(b)
	shared := 0
	for t := range ta {
		if tb[t] {
			shared++
		}
	}
	return float64(shared) / float64(len(ta)+len(tb)-shared)
}

func trigrams(w string) map[string]bool {
	r := []rune("  " + w + " ")
	out := map[string]bool{}
	for i := 0; i+3 <= len(r); i++ {
		out[string(r[i:i+3])] = true
	}
	return out
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
