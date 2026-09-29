package ask

import (
	"strings"
	"testing"
)

// A why question, in English or Indonesian, ends with the reason rule again;
// other questions do not.
func TestUserRepeatsTheReasonRuleForWhyQuestions(t *testing.T) {
	for q, want := range map[string]bool{
		"Why are trips across midnight counted on the departure date?": true,
		"Kenapa slip gaji tidak dikirim lewat WhatsApp?":               true,
		"Mengapa periode payroll diubah?":                              true,
		"Apa alasannya approval lembur dilewati?":                      true,
		"What is the payroll period now?":                              false,
		"Siapa yang meminta perubahan periode payroll?":                false,
		"Bagaimana cara kerja potongan pinjaman?":                      false,
	} {
		got := strings.Contains(User(q, "[HRIS-1] Ticket", ""), "The question asks why.")
		if got != want {
			t.Errorf("%q: reminder %v, want %v", q, got, want)
		}
	}
}
