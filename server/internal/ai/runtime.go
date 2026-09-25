package ai

import (
	"errors"
	"net/http"

	"github.com/kenzo03/muasal/server/internal/llm"
	"github.com/kenzo03/muasal/server/internal/secret"
)

// ErrOff means AI is switched off, so no client exists (§13.4).
var ErrOff = errors.New("AI is turned off")

// Runtime is what the API and the workers share: the settings store, the
// generation gate, the key that opens stored API keys, and the HTTP client
// for model servers.
type Runtime struct {
	Store     *Store
	Gate      *Gate
	SecretKey []byte
	HTTP      *http.Client
}

// ChatClient builds the chat client the settings describe.
func (r *Runtime) ChatClient(s Settings) (*llm.Client, error) { return r.client(s, s.Chat) }

// EmbedClient builds the embedding client the settings describe.
func (r *Runtime) EmbedClient(s Settings) (*llm.Client, error) { return r.client(s, s.Embed) }

func (r *Runtime) client(s Settings, e Endpoint) (*llm.Client, error) {
	if s.Mode == ModeOff {
		return nil, ErrOff
	}
	key := ""
	if len(e.SealedKey) > 0 {
		plain, err := secret.Open(r.SecretKey, e.SealedKey)
		if err != nil {
			return nil, err
		}
		key = string(plain)
	}
	return llm.New(e.URL, e.Model, key, r.HTTP), nil
}
