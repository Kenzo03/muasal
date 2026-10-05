package ask_test

import (
	"slices"
	"testing"

	"github.com/kenzo03/zettra/server/internal/ask"
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

// MSL-5: a one-digit figure is no evidence, since dates, versions and keys all
// hold one. The 1 of "H+1" once moved a claim onto the ticket it contradicted.
func TestReciteIgnoresOneDigitFigures(t *testing.T) {
	blocks := map[string]string{
		"DMS-2":  "[DMS-2] Bug · open, created 2026-09-29\nTitle: Surat jalan mencetak alamat gudang pusat",
		"DMS-13": "[DMS-13] Feature · closed 2025-06-10\nTitle: Jadwal pengiriman H+1 setelah SO disetujui",
	}
	c := ask.Claim{Text: "Tidak ada catatan yang menyatakan alasan pengiriman H+1.", Cites: []string{"DMS-2"}}
	if got, moved := ask.Recite(c, blocks, []string{"DMS-13", "DMS-2"}); moved != nil || !slices.Equal(got.Cites, c.Cites) {
		t.Errorf("cites %v moved %+v, want the claim unchanged", got.Cites, moved)
	}
}

// MSL-44: a one-digit figure with its unit is checked; the claim moves to the
// section that states it, and a bare digit such as the 1 of H+1 still is not.
func TestReciteChecksOneDigitFiguresWithUnits(t *testing.T) {
	blocks := map[string]string{
		"HRIS-DOC1/2.1": "[HRIS-DOC1/2.1] Pengajuan Lembur (OT-01)\nLembur diajukan sebelum dikerjakan. Maksimal 3 jam per hari dan 14 jam per minggu.",
		"HRIS-DOC1/2.2": "[HRIS-DOC1/2.2] Persetujuan Lembur (OT-02)\nLembur disetujui oleh Kepala Toko, lalu Area Manager bila lebih dari 2 jam dalam satu hari.",
		"HRIS-DOC1/3.1": "[HRIS-DOC1/3.1] Pengajuan Cuti (LV-01)\nHak cuti tahunan 12 hari; sisa cuti maksimal 3 hari dibawa.",
	}
	order := []string{"HRIS-DOC1/2.2", "HRIS-DOC1/3.1", "HRIS-DOC1/2.1"}
	c := ask.Claim{Text: "Maksimal jam lembur per hari adalah 3 jam.", Cites: []string{"HRIS-DOC1/2.2"}}
	if got, moved := ask.Recite(c, blocks, order); moved == nil || !slices.Equal(got.Cites, []string{"HRIS-DOC1/2.1"}) {
		t.Errorf("cites %v moved %+v, want HRIS-DOC1/2.1 (3 jam, not 3 hari)", got.Cites, moved)
	}
	ok := ask.Claim{Text: "Area Manager menyetujui lembur lebih dari 2 jam.", Cites: []string{"HRIS-DOC1/2.2"}}
	if got, moved := ask.Recite(ok, blocks, order); moved != nil || !slices.Equal(got.Cites, ok.Cites) {
		t.Errorf("a right citation moved: %v %+v", got.Cites, moved)
	}
}

// MSL-43: a claim that calls a ticket late whose due note says otherwise goes;
// a true one, or one saying it isn't late, stays.
func TestContradictsDue(t *testing.T) {
	blocks := map[string]string{
		"HRIS-8": "[HRIS-8] Bug · open\nAssigned to Fajar · priority urgent · due 2026-10-01, due tomorrow, not overdue\n",
		"HRIS-6": "[HRIS-6] Bug · open\nAssigned to Fajar · priority urgent · due 2026-09-28, overdue by 2 days\n",
	}
	for text, want := range map[string]bool{
		"Tiket HRIS-8 terlambat karena jatuh tempo besok.":  true,
		"HRIS-6 is overdue by two days.":                    false,
		"HRIS-8 belum terlambat; jatuh tempo besok.":        false,
		"HRIS-8 dipegang Fajar Nugroho.":                    false,
		"Both HRIS-6 and HRIS-8 are late, both with Fajar.": true,
	} {
		if got := ask.ContradictsDue(ask.Claim{Text: text}, blocks); got != want {
			t.Errorf("%q: %v, want %v", text, got, want)
		}
	}
	// The live answer's third claim named no key but cited the one not overdue.
	if !ask.ContradictsDue(ask.Claim{Text: "Ketiga tiket terlambat tersebut dipegang oleh Fajar Nugroho.", Cites: []string{"HRIS-6", "HRIS-8"}}, blocks) {
		t.Error("a cited not-overdue ticket should count")
	}
}
