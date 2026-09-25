package ask

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Claim is one sentence or bullet of an answer with its citations.
type Claim struct {
	Text  string   `json:"text"`
	Cites []string `json:"cites"`
}

// Claims reads a streamed answer, {"claims":[{...},{...}]}, and calls emit for
// each claim as soon as its object closes, so the first claim shows before the
// model finishes (§11.5). Text before the first "{" is skipped, for providers
// that wrap JSON mode output. It returns an error for invalid JSON, after
// emitting the claims that came before it.
func Claims(r io.Reader, emit func(Claim)) error {
	br := bufio.NewReader(r)
	for {
		b, err := br.ReadByte()
		if err != nil {
			return fmt.Errorf("no JSON object in the answer: %w", err)
		}
		if b == '{' {
			_ = br.UnreadByte()
			break
		}
	}
	dec := json.NewDecoder(br)
	if err := expect(dec, json.Delim('{')); err != nil {
		return err
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		if tok != "claims" {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return err
			}
			continue
		}
		if err := expect(dec, json.Delim('[')); err != nil {
			return err
		}
		for dec.More() {
			var c Claim
			if err := dec.Decode(&c); err != nil {
				return err
			}
			emit(c)
		}
		if err := expect(dec, json.Delim(']')); err != nil {
			return err
		}
	}
	return expect(dec, json.Delim('}'))
}

func expect(dec *json.Decoder, want json.Delim) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok != want {
		return errors.New("unexpected JSON in the answer")
	}
	return nil
}
