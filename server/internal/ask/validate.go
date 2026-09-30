package ask

import (
	"regexp"
	"slices"
	"strings"
)

// Dropped records what validation removed and why, for the Ask log (§10.8).
type Dropped struct {
	Claim   Claim    `json:"claim"`
	Reason  string   `json:"reason"`            // no_citation, outside_key, empty, citation_removed, citation_unsupported, contradicts_reason
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
		k = strings.TrimSpace(k)
		switch e := evidenceKey(evidence, k); {
		case e == "":
			removed = append(removed, strings.ToUpper(k))
		case !slices.Contains(out.Cites, e):
			out.Cites = append(out.Cites, e)
		}
	}
	switch {
	case out.Text == "":
		return Claim{}, &Dropped{Claim: c, Reason: "empty", Removed: removed}
	case len(out.Cites) == 0:
		return Claim{}, &Dropped{Claim: c, Reason: "no_citation", Removed: removed}
	}
	for _, m := range keyRe.FindAllStringSubmatch(out.Text, -1) {
		if evidenceKey(evidence, m[1]) == "" {
			return Claim{}, &Dropped{Claim: c, Reason: "outside_key", Removed: removed}
		}
	}
	if len(removed) > 0 {
		return out, &Dropped{Claim: c, Reason: "citation_removed", Removed: removed}
	}
	return out, nil
}

// evidenceKey returns the packed key k names, ignoring case, or "". Keys
// keep their spelling: a section under an unnumbered heading is DOC1/s3.
func evidenceKey(evidence []string, k string) string {
	for _, e := range evidence {
		if strings.EqualFold(e, k) {
			return e
		}
	}
	return ""
}

var (
	// figureRe finds the figures a text states: days, amounts, times, shares.
	figureRe = regexp.MustCompile(`\d+`)
	// unitRe finds a one-digit figure that carries its unit, such as "3 jam"
	// (MSL-44); the 1 of "H+1" carries none.
	unitRe = regexp.MustCompile(`(?i)(?:^|[^\d.,])(\d)\s*(%|persen|percent|jam|hari|minggu|bulan|tahun|menit|kali|orang|toko|hours?|days?|weeks?|months?|years?|minutes?|times?)\b`)
	// citeKeyRe finds citation keys in a claim's text, whose digits are no figures:
	// HRIS-231, HRIS-DN7, HRIS-DOC1/7.4, and FSD menu IDs such as PAY-PR-03.
	citeKeyRe = regexp.MustCompile(`(?i)\b[a-z][a-z0-9]{1,9}-(?:doc\d+/[\w.]+|dn\d+|[a-z]{1,5}-\d+|\d+)\b`)
)

var (
	// noReasonRe finds a claim that the evidence records no reason, as the
	// small model writes it in Indonesian or English.
	noReasonRe = regexp.MustCompile(`(?i)\b(tidak|belum|tak)\b.{0,60}\b(dicatat|tercatat|disebut\w*|menyebut\w*|menyatakan|dinyatakan|menjelaskan|dijelaskan|ada catatan)\b.{0,60}\b(alasan\w*|mengapa|kenapa)\b` +
		`|\balasan\w*\b.{0,40}\b(tidak|belum|tak)\b.{0,20}\b(dicatat|tercatat|disebut\w*|dinyatakan|dijelaskan|ada)\b` +
		`|\b(tidak|belum|tak)\s+ada\s+alasan\b|\bno\s+(reason|explanation)\b` +
		`|\b(does not|doesn't|do not|don't|did not|didn't|not)\b.{0,40}\b(say|says|state|states|stated|record|records|recorded|explain|explains|mention|mentions|give|gives)\b.{0,40}\b(why|reason)\b` +
		`|\breasons?\b.{0,30}\b(is|are|was|were)\s+not\s+(recorded|stated|given|documented|mentioned)\b`)
	// becauseRe finds a claim that gives a reason.
	// lateRe and notLateRe spot a claim that calls a ticket late, and one that
	// says it is not (MSL-43).
	lateRe    = regexp.MustCompile(`(?i)\b(terlambat|telat|overdue|late|past due|lewat jatuh tempo|melewati (?:batas waktu|tenggat|jatuh tempo))\b`)
	notLateRe = regexp.MustCompile(`(?i)\b(belum|tidak|tak|bukan|not|isn't|is not|no longer|tidak lagi)\s+(terlambat|telat|overdue|late|past due)\b`)
	becauseRe = regexp.MustCompile(`(?i)\b(karena|sebab|disebabkan|akibat|agar|supaya|because|due to|so that)\b`)
)

// saysNoReason reports whether a claim says the reason is not recorded.
func saysNoReason(text string) bool { return noReasonRe.MatchString(text) }

// ContradictsDue reports whether a claim calls a ticket late whose own due note
// says it is not: a small model does so even with "due tomorrow, not overdue"
// in front of it (MSL-43).
func ContradictsDue(c Claim, blocks map[string]string) bool {
	if !lateRe.MatchString(c.Text) || notLateRe.MatchString(c.Text) {
		return false
	}
	for _, key := range citeKeyRe.FindAllString(c.Text, -1) {
		for k, b := range blocks {
			if strings.EqualFold(k, key) && (strings.Contains(b, ", not overdue") || strings.Contains(b, ", due today")) {
				return true
			}
		}
	}
	return false
}

// givesReason reports whether a claim gives a reason.
func givesReason(text string) bool { return becauseRe.MatchString(text) && !saysNoReason(text) }

// figures lists a text's numbers of two digits or more, without leading
// zeros, so 09 and 9 match. A bare digit is no evidence: dates, versions and
// keys all hold one, as does the 1 of "H+1" (MSL-5). A digit with its unit is:
// "3 jam" is the rule, kept as "3 jam" so it never matches "3 hari" (MSL-44).
func figures(s string) map[string]bool {
	out := map[string]bool{}
	for _, n := range figureRe.FindAllString(s, -1) {
		if t := strings.TrimLeft(n, "0"); len(t) > 1 {
			out[t] = true
		}
	}
	for _, m := range unitRe.FindAllStringSubmatch(s, -1) {
		unit := strings.ToLower(m[2])
		if len(unit) > 3 && strings.HasSuffix(unit, "s") {
			unit = strings.TrimSuffix(unit, "s") // hours and hour are one unit
		}
		out[m[1]+" "+unit] = true
	}
	return out
}

// Recite checks the figures a claim states against what it cites (§11.5).
// A model can cite the item a topic comes from rather than the one stating
// the figure, such as the note that set the old payroll period for a claim
// about the new one. A cited item holding none of the claim's figures loses
// the citation; a claim left without any moves to the packed items holding
// all of its figures (the first two, by rank), and keeps what the model cited
// when none does. Claims without figures pass as they are.
func Recite(c Claim, blocks map[string]string, order []string) (Claim, *Dropped) {
	want := figures(citeKeyRe.ReplaceAllString(c.Text, ""))
	if len(want) == 0 {
		return c, nil
	}
	stated := func(key string) int { // how many of the claim's figures the item holds
		have, n := figures(blocks[key]), 0
		for f := range want {
			if have[f] {
				n++
			}
		}
		return n
	}
	var kept, removed []string
	for _, k := range c.Cites {
		if stated(k) > 0 {
			kept = append(kept, k)
		} else {
			removed = append(removed, k)
		}
	}
	if len(removed) == 0 {
		return c, nil
	}
	if len(kept) == 0 {
		for _, k := range order {
			if len(kept) < 2 && stated(k) == len(want) {
				kept = append(kept, k)
			}
		}
	}
	if len(kept) == 0 {
		return c, nil
	}
	return Claim{Text: c.Text, Cites: kept}, &Dropped{Claim: c, Reason: "citation_unsupported", Removed: removed}
}
