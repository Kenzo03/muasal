package ask

import (
	"slices"
	"strings"
)

// Dropped records what validation removed and why, for the Ask log (§10.8).
type Dropped struct {
	Claim   Claim    `json:"claim"`
	Reason  string   `json:"reason"`            // no_citation, outside_key, empty
	Removed []string `json:"removed,omitempty"` // citations outside the evidence
}

// Validate checks one claim against the evidence keys. The server trusts
// neither the model server's grammar nor a cloud provider's (§11.5):
// citations outside the evidence are dropped; a claim left without citations
// is dropped; so is a claim whose text names a key outside the evidence.
func Validate(c Claim, evidence []string) (Claim, *Dropped) {
	out := Claim{Text: strings.TrimSpace(c.Text)}
	if r := []rune(out.Text); len(r) > 400 {
		out.Text = string(r[:400])
	}
	var removed []string
	for _, k := range c.Cites {
		k = strings.ToUpper(strings.TrimSpace(k))
		switch {
		case !slices.Contains(evidence, k):
			removed = append(removed, k)
		case !slices.Contains(out.Cites, k):
			out.Cites = append(out.Cites, k)
		}
	}
	switch {
	case out.Text == "":
		return Claim{}, &Dropped{Claim: c, Reason: "empty", Removed: removed}
	case len(out.Cites) == 0:
		return Claim{}, &Dropped{Claim: c, Reason: "no_citation", Removed: removed}
	}
	for _, m := range keyRe.FindAllStringSubmatch(out.Text, -1) {
		if !slices.Contains(evidence, strings.ToUpper(m[1])) {
			return Claim{}, &Dropped{Claim: c, Reason: "outside_key", Removed: removed}
		}
	}
	if len(removed) > 0 {
		return out, &Dropped{Claim: c, Reason: "citation_removed", Removed: removed}
	}
	return out, nil
}
