// Package ai holds the AI settings an admin picks in Admin → AI, the gate that
// limits concurrent generations, and the model clients those settings describe
// (FSD §11.7, §13.4). AI is optional: in Off mode nothing here calls a model.
package ai

import (
	"fmt"
	"net/url"
	"strings"
)

// Mode is who runs the models: nobody, the customer's own server, or a cloud
// provider with the customer's key (§13.4).
type Mode string

const (
	ModeOff   Mode = "off"
	ModeLocal Mode = "local"
	ModeBYOK  Mode = "byok"
)

// Endpoint is one OpenAI-compatible API: its base URL (ending in /v1), the
// model to use there, and the API key sealed with APP_SECRET_KEY (R-AI-3).
type Endpoint struct {
	URL       string `json:"url"`
	Model     string `json:"model"`
	SealedKey []byte `json:"sealed_key,omitempty"`
}

// Settings is the `ai` row of the settings table. Chat and embeddings are set
// apart, because keeping embeddings local sends only each question's evidence
// out (R-AI-2).
type Settings struct {
	Mode         Mode     `json:"mode"`
	Provider     string   `json:"provider"`     // BYOK: the provider's name, shown in the badge and the acknowledgement
	Acknowledged bool     `json:"acknowledged"` // R-AI-1: the admin accepted that evidence goes to Provider
	Chat         Endpoint `json:"chat"`
	Embed        Endpoint `json:"embed"`
	EmbedDim     int      `json:"embed_dim"` // detected by Test connection; the chunks column follows it

	ContextTokens  int     `json:"context_tokens"`  // evidence budget (§11.4)
	MaxConcurrent  int     `json:"max_concurrent"`  // generations at once (§11.7)
	Temperature    float64 `json:"temperature"`     // §11.5
	TimeoutSeconds int     `json:"timeout_seconds"` // per answer (§11.5)
	MinSimilarity  float64 `json:"min_similarity"`  // relevance floor (§11.3)
	ExhaustiveMax  int     `json:"exhaustive_max"`  // small sets skip ranking (§11.3)
}

// Defaults is a fresh install: AI Off until the installer picks Local or BYOK,
// with the dev-laptop tier's presets ready for Local (§18.1). The model server
// in the compose stack is reached as http://model:11434/v1.
func Defaults() Settings {
	return Settings{
		Mode:           ModeOff,
		Chat:           Endpoint{URL: "http://model:11434/v1", Model: "qwen3.5:4b"},
		Embed:          Endpoint{URL: "http://model:11434/v1", Model: "bge-m3"},
		EmbedDim:       1024,
		ContextTokens:  2500,
		MaxConcurrent:  1,
		Temperature:    0.1,
		TimeoutSeconds: 60,
		MinSimilarity:  0.45,
		ExhaustiveMax:  40,
	}
}

// Problem is one invalid field, named as the API names it.
type Problem struct{ Field, Code, Message string }

// Validate checks the settings as they will be saved. BYOK needs the
// acknowledgement (R-AI-1) and a chat key.
func (s Settings) Validate() []Problem {
	var p []Problem
	switch s.Mode {
	case ModeOff, ModeLocal, ModeBYOK:
	default:
		return []Problem{{"mode", "invalid", "Choose Off, Local or Bring your own key"}}
	}
	if s.Mode == ModeOff {
		return nil
	}
	for _, e := range []struct {
		name string
		ep   Endpoint
	}{{"chat", s.Chat}, {"embed", s.Embed}} {
		if u, err := url.Parse(e.ep.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			p = append(p, Problem{e.name + ".url", "invalid", "Enter the API's base URL, e.g. http://model:11434/v1"})
		}
		if strings.TrimSpace(e.ep.Model) == "" {
			p = append(p, Problem{e.name + ".model", "required", "Enter a model name"})
		}
	}
	if s.Mode == ModeBYOK {
		if strings.TrimSpace(s.Provider) == "" {
			p = append(p, Problem{"provider", "required", "Name the provider, e.g. OpenAI"})
		}
		if len(s.Chat.SealedKey) == 0 {
			p = append(p, Problem{"chat.api_key", "required", "Enter the provider's API key"})
		}
		if !s.Acknowledged {
			p = append(p, Problem{"acknowledged", "byok_not_acknowledged", fmt.Sprintf("Confirm that questions and ticket excerpts will be sent to %s", s.providerName())})
		}
	}
	for _, r := range []struct {
		field   string
		ok      bool
		message string
	}{
		{"context_tokens", s.ContextTokens >= 500 && s.ContextTokens <= 32000, "Use 500 to 32,000 tokens"},
		{"max_concurrent", s.MaxConcurrent >= 1 && s.MaxConcurrent <= 16, "Use 1 to 16"},
		{"temperature", s.Temperature >= 0 && s.Temperature <= 1, "Use 0 to 1"},
		{"timeout_seconds", s.TimeoutSeconds >= 10 && s.TimeoutSeconds <= 300, "Use 10 to 300 seconds"},
		{"min_similarity", s.MinSimilarity >= 0 && s.MinSimilarity < 1, "Use 0 to 0.99"},
		{"exhaustive_max", s.ExhaustiveMax >= 0 && s.ExhaustiveMax <= 200, "Use 0 to 200"},
	} {
		if !r.ok {
			p = append(p, Problem{r.field, "invalid", r.message})
		}
	}
	return p
}

// Badge names the chat model as answers show it (§10.3): "Local · qwen3.5:4b"
// or "Cloud · OpenAI · gpt-5-mini".
func (s Settings) Badge() string {
	if s.Mode == ModeBYOK {
		return "Cloud · " + s.providerName() + " · " + s.Chat.Model
	}
	return "Local · " + s.Chat.Model
}

func (s Settings) providerName() string {
	if p := strings.TrimSpace(s.Provider); p != "" {
		return p
	}
	return "the provider"
}
