package docs

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// Node is one proposed node of a tree draft. Parent names another node's
// TmpID, or is empty at the top. Exists marks a node the tree already has,
// matched by path; applying leaves it unchanged. Duplicate names a sibling
// whose name is nearly the same (trigram similarity ≥ 0.8).
type Node struct {
	TmpID       string   `json:"tmp_id"`
	Parent      string   `json:"parent"`
	Type        string   `json:"type"` // module or menu
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases"`
	Description string   `json:"description"`
	Sections    []string `json:"sections"`
	Keep        bool     `json:"keep"`
	Exists      bool     `json:"exists"`
	Duplicate   string   `json:"duplicate,omitempty"`
}

// Candidate is one node the model (or a heading) proposes, by path.
type Candidate struct {
	Path        []string
	Type        string
	Aliases     []string
	Description string
	Section     string
}

// FromHeadings proposes a tree without a model (R-MR-11): the shallowest two
// heading levels below the title become modules and the menus under them.
// With the usual "#" title, those are "##" and "###".
func FromHeadings(sections []Section) []Candidate {
	levels := []int{}
	for _, s := range sections {
		if s.Number != "0" && !slices.Contains(levels, s.Level) {
			levels = append(levels, s.Level)
		}
	}
	slices.Sort(levels)
	if len(levels) > 2 && countLevel(sections, levels[0]) == 1 {
		levels = levels[1:] // a lone top heading is the document's title
	}
	if len(levels) == 0 {
		return nil
	}
	top := levels[0]
	var out []Candidate
	var module string
	for _, s := range sections {
		switch {
		case s.Number == "0":
		case s.Level == top:
			module = s.Title
			out = append(out, Candidate{Path: []string{s.Title}, Type: "module", Section: s.Number})
		case len(levels) > 1 && s.Level == levels[1] && module != "":
			out = append(out, Candidate{Path: []string{module, s.Title}, Type: "menu", Section: s.Number})
		}
	}
	return out
}

func countLevel(sections []Section, level int) int {
	n := 0
	for _, s := range sections {
		if s.Level == level && s.Number != "0" {
			n++
		}
	}
	return n
}

// Existing is a live node of the project's tree, by path.
type Existing struct {
	ID   int64
	Path []string
}

// Merge turns candidates into a proposal without a model call (§7.7):
// paths are normalized and merged, missing parents become modules, nodes the
// tree has are marked Exists, and near-duplicate siblings are flagged.
func Merge(cands []Candidate, existing []Existing) []Node {
	var out []Node
	byPath := map[string]int{} // normalized path → index in out
	have := map[string]bool{}
	for _, e := range existing {
		have[pathKey(e.Path)] = true
	}
	var add func(path []string, typ string) int
	add = func(path []string, typ string) int {
		k := pathKey(path)
		if i, ok := byPath[k]; ok {
			if typ == "module" {
				out[i].Type = "module" // a node with children is a module
			}
			return i
		}
		parent := ""
		if len(path) > 1 {
			parent = out[add(path[:len(path)-1], "module")].TmpID
		}
		out = append(out, Node{TmpID: fmt.Sprintf("n%d", len(out)+1), Parent: parent, Type: typ, Name: path[len(path)-1],
			Aliases: []string{}, Sections: []string{}, Keep: !have[k], Exists: have[k]})
		byPath[k] = len(out) - 1
		return len(out) - 1
	}
	for _, c := range cands {
		path := clean(c.Path)
		if len(path) == 0 {
			continue
		}
		typ := c.Type
		if typ != "module" {
			typ = "menu"
		}
		i := add(path, typ)
		n := &out[i]
		for _, a := range c.Aliases {
			if a = norm(a); a != "" && !strings.EqualFold(a, n.Name) && !containsFold(n.Aliases, a) && len(n.Aliases) < 3 {
				n.Aliases = append(n.Aliases, a)
			}
		}
		if n.Description == "" {
			n.Description = cut(strings.TrimSpace(c.Description), 300)
		}
		if c.Section != "" && !slices.Contains(n.Sections, c.Section) {
			n.Sections = append(n.Sections, c.Section)
		}
	}
	// Near-duplicate siblings: the later one names the earlier.
	for i := range out {
		for j := range i {
			if out[i].Parent == out[j].Parent && Similarity(out[i].Name, out[j].Name) >= 0.8 {
				out[i].Duplicate = out[j].TmpID
				break
			}
		}
	}
	return out
}

// Paths returns each node's path by TmpID.
func Paths(nodes []Node) map[string][]string {
	byID := map[string]Node{}
	for _, n := range nodes {
		byID[n.TmpID] = n
	}
	out := map[string][]string{}
	var path func(id string, depth int) []string
	path = func(id string, depth int) []string {
		n, ok := byID[id]
		if !ok || depth > 50 {
			return nil
		}
		if p, ok := out[id]; ok {
			return p
		}
		p := append(slices.Clone(path(n.Parent, depth+1)), n.Name)
		out[id] = p
		return p
	}
	for _, n := range nodes {
		path(n.TmpID, 0)
	}
	return out
}

func clean(path []string) []string {
	var out []string
	for _, p := range path {
		if p = norm(p); p != "" {
			out = append(out, cut(p, 200))
		}
	}
	return out
}

// norm trims and collapses spaces.
func norm(s string) string { return strings.Join(strings.Fields(s), " ") }

func pathKey(path []string) string {
	parts := make([]string, len(path))
	for i, p := range path {
		parts[i] = strings.ToLower(norm(p))
	}
	return strings.Join(parts, "\x00")
}

func containsFold(list []string, s string) bool {
	return slices.ContainsFunc(list, func(x string) bool { return strings.EqualFold(x, s) })
}

func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// Similarity is pg_trgm's trigram similarity: words padded with two spaces
// in front and one behind, lower-cased, compared as sets of three-letter runs.
func Similarity(a, b string) float64 {
	ta, tb := trigrams(a), trigrams(b)
	if len(ta) == 0 && len(tb) == 0 {
		return 1
	}
	shared := 0
	for t := range ta {
		if tb[t] {
			shared++
		}
	}
	return float64(shared) / float64(len(ta)+len(tb)-shared)
}

func trigrams(s string) map[string]bool {
	out := map[string]bool{}
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for _, w := range words {
		r := []rune("  " + w + " ")
		for i := 0; i+3 <= len(r); i++ {
			out[string(r[i:i+3])] = true
		}
	}
	return out
}
