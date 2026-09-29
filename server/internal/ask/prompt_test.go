package ask

import (
	"strings"
	"testing"
)

// MSL-4: the claims a why answer holds back, and the ones that give a reason,
// as the small model wrote them.
func TestReasonClaims(t *testing.T) {
	for text, want := range map[string][2]bool{ // saysNoReason, givesReason
		"Meskipun DMS-2 menjelaskan bahwa alamat gudang yang salah menyebabkan keterlambatan, tidak ada catatan spesifik yang menyatakan alasan mengapa pengiriman dijadwalkan H+1.": {true, false},
		"Dokumen spesifikasi fungsional tidak menjelaskan alasan mengapa pengiriman dijadwalkan H+1 setelah SO disetujui.":                                                           {true, false},
		"Alasannya tidak dicatat.":                                                {true, false},
		"Tidak ada alasan yang dicatat untuk aturan ini.":                         {true, false},
		"The evidence does not say why HR approves overtime.":                     {true, false},
		"The reason is not recorded.":                                             {true, false},
		"Pengiriman dijadwalkan H+1 setelah SO disetujui karena armada terbatas.": {false, true},
		"HR approves overtime because supervisors are on leave.":                  {false, true},
		"Pengiriman dijadwalkan H+1 setelah SO disetujui.":                        {false, false},
		"The threshold has been Rp 50 juta since October.":                        {false, false},
	} {
		if got := [2]bool{saysNoReason(text), givesReason(text)}; got != want {
			t.Errorf("%q: %v, want %v", text, got, want)
		}
	}
}

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
