package ask_test

import (
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/ask"
)

// §11.5: a claim's figures decide what it may cite. A cited item holding none
// of them loses the citation, and a claim left with none moves to the item
// that states them all; otherwise claims keep what the model cited.
func TestReciteMovesACitationToWhatStatesTheFigures(t *testing.T) {
	blocks := map[string]string{
		"PAY-DN1":        "[PAY-DN1] Decision note · decided 2026-09-29\nPeriode payroll tanggal 21 sampai tanggal 20.",
		"PAY-DOC2/3.2.1": "[PAY-DOC2/3.2.1] Document section · uploaded 2026-09-29\nPeriode payroll berjalan tanggal 21 sampai tanggal 20.",
		"PAY-4":          "[PAY-4] Change request · closed 2026-09-29\nDecision (implemented): diubah dari tanggal 21-20 menjadi tanggal 26-25.",
	}
	order := []string{"PAY-DN1", "PAY-DOC2/3.2.1", "PAY-4"}
	for _, c := range []struct {
		text        string
		cites, want []string
		moved       bool
	}{
		{"Periode sekarang tanggal 26 sampai 25.", []string{"PAY-DN1", "PAY-DOC2/3.2.1"}, []string{"PAY-4"}, true},
		{"Sebelumnya tanggal 21 sampai 20.", []string{"PAY-DN1", "PAY-DOC2/3.2.1"}, []string{"PAY-DN1", "PAY-DOC2/3.2.1"}, false},
		{"Diubah dari 21-20 menjadi 26-25.", []string{"PAY-4", "PAY-DN1"}, []string{"PAY-4", "PAY-DN1"}, false},
		{"Rahmat meminta perubahan ini.", []string{"PAY-DN1"}, []string{"PAY-DN1"}, false},
		{"Ada 7 tiket terbuka.", []string{"PAY-DN1"}, []string{"PAY-DN1"}, false},
		{"PAY-4 dan PAY-DOC2/3.2.1 membahas periodenya.", []string{"PAY-4"}, []string{"PAY-4"}, false},
	} {
		got, moved := ask.Recite(ask.Claim{Text: c.text, Cites: c.cites}, blocks, order)
		if !slices.Equal(got.Cites, c.want) || (moved != nil) != c.moved || got.Text != c.text {
			t.Errorf("%q: cites %v moved %v, want %v moved %v", c.text, got.Cites, moved != nil, c.want, c.moved)
		}
	}
}
