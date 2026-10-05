package ai_test

import (
	"testing"

	"github.com/kenzo03/zettra/server/internal/ai"
)

// A fresh install runs without AI until the installer picks a mode (§13.4).
func TestDefaultsAreOffWithLaptopPresets(t *testing.T) {
	s := ai.Defaults()
	if s.Mode != ai.ModeOff || s.Chat.Model != "qwen3.5:4b" || s.Embed.Model != "bge-m3" || s.EmbedDim != 1024 ||
		s.ContextTokens != 2500 || s.MaxConcurrent != 1 || s.ExhaustiveMax != 40 || len(s.Validate()) != 0 {
		t.Fatalf("defaults: %+v %v", s, s.Validate())
	}
}

// R-AI-1: BYOK needs a provider, a key and the acknowledgement.
func TestBYOKNeedsTheAcknowledgement(t *testing.T) {
	s := ai.Defaults()
	s.Mode = ai.ModeBYOK
	s.Chat = ai.Endpoint{URL: "https://api.example.com/v1", Model: "gpt-5-mini"}
	var fields []string
	for _, p := range s.Validate() {
		fields = append(fields, p.Field+":"+p.Code)
	}
	want := []string{"provider:required", "chat.api_key:required", "acknowledged:byok_not_acknowledged"}
	if len(fields) != len(want) {
		t.Fatalf("problems: %v, want %v", fields, want)
	}
	for i := range want {
		if fields[i] != want[i] {
			t.Fatalf("problems: %v, want %v", fields, want)
		}
	}
	s.Provider, s.Chat.SealedKey, s.Acknowledged = "OpenAI", []byte("sealed"), true
	if p := s.Validate(); len(p) != 0 || s.Badge() != "Cloud · OpenAI · gpt-5-mini" {
		t.Fatalf("complete BYOK: %v %q", p, s.Badge())
	}
}

func TestLocalNeedsURLsAndModels(t *testing.T) {
	s := ai.Defaults()
	s.Mode = ai.ModeLocal
	s.Chat.URL, s.Embed.Model, s.MaxConcurrent = "model:11434", "", 0
	var fields []string
	for _, p := range s.Validate() {
		fields = append(fields, p.Field)
	}
	if len(fields) != 3 || fields[0] != "chat.url" || fields[1] != "embed.model" || fields[2] != "max_concurrent" {
		t.Fatalf("problems: %v", fields)
	}
	if ai.Defaults().Badge() != "Local · qwen3.5:4b" {
		t.Fatal("local badge")
	}
}

// §18.1: the installer switches to Local with the chosen tier's presets.
func TestLocalForTier(t *testing.T) {
	for tier, want := range map[string]struct {
		chat          string
		budget, slots int
	}{
		"dev": {"qwen3.5:4b", 2500, 1}, "minimum": {"qwen3.5:4b", 2500, 1}, "recommended": {"qwen3.5:9b", 6000, 2},
	} {
		s, err := ai.Defaults().LocalForTier("http://model:11434/v1", tier)
		if err != nil || s.Mode != ai.ModeLocal || s.Chat.Model != want.chat || s.Embed.Model != "bge-m3" || s.Chat.URL != "http://model:11434/v1" ||
			s.ContextTokens != want.budget || s.MaxConcurrent != want.slots || len(s.Validate()) != 0 {
			t.Errorf("%s: %+v %v", tier, s, err)
		}
	}
	if _, err := ai.Defaults().LocalForTier("http://model:11434/v1", "huge"); err == nil {
		t.Error("an unknown tier should fail")
	}
}
