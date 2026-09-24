package embedding

import (
	"fmt"
	"strings"

	"github.com/tiktoken-go/tokenizer"
)

// TokenSplitter counts BPE tokens and preserves the document header on every chunk.
// cl100k_base is a counting tokenizer; the embedding provider may tokenize
// differently, so its own maximum input length must still be respected.
type TokenSplitter struct {
	codec   tokenizer.Codec
	size    int
	overlap int
}

func NewTokenSplitter(size, overlap int) (*TokenSplitter, error) {
	if size < 1 || overlap < 0 || overlap >= size {
		return nil, fmt.Errorf("invalid token chunk size or overlap")
	}
	codec, err := tokenizer.Get(tokenizer.Cl100kBase)
	if err != nil {
		return nil, fmt.Errorf("load cl100k_base tokenizer: %w", err)
	}
	return &TokenSplitter{codec: codec, size: size, overlap: overlap}, nil
}

func (s *TokenSplitter) count(text string) (int, error) {
	ids, _, err := s.codec.Encode(text)
	return len(ids), err
}

func (s *TokenSplitter) Split(doc Document) ([]string, error) {
	if strings.TrimSpace(doc.PageContent) == "" {
		return nil, nil
	}
	headerTokens, err := s.count(doc.Header)
	if err != nil {
		return nil, err
	}
	// Leave room for BPE merges where the repeated header meets the body.
	limit := s.size - headerTokens - 4
	if limit <= 0 || s.overlap >= limit {
		return nil, fmt.Errorf("document header leaves insufficient token budget")
	}
	parts, err := s.splitParts(doc.PageContent, limit, []string{"\n\n", "\n", ". ", " ", ""})
	if err != nil {
		return nil, err
	}
	var chunks []string
	current := ""
	for _, part := range parts {
		candidate := current + part
		count, err := s.count(doc.Header + candidate)
		if err != nil {
			return nil, err
		}
		if count <= s.size {
			current = candidate
			continue
		}
		if strings.TrimSpace(current) != "" {
			chunks = append(chunks, doc.Header+strings.TrimSpace(current))
		}
		current = part
		if s.overlap > 0 && len(chunks) > 0 {
			previous := strings.TrimPrefix(chunks[len(chunks)-1], doc.Header)
			ids, _, err := s.codec.Encode(previous)
			if err != nil {
				return nil, err
			}
			if len(ids) > s.overlap {
				ids = ids[len(ids)-s.overlap:]
			}
			for len(ids) > 0 {
				tail, err := s.codec.Decode(ids)
				if err != nil {
					return nil, err
				}
				candidate = tail + current
				count, err := s.count(doc.Header + candidate)
				if err != nil {
					return nil, err
				}
				if count <= s.size {
					current = candidate
					break
				}
				ids = ids[1:]
			}
		}
	}
	if strings.TrimSpace(current) != "" {
		chunks = append(chunks, doc.Header+strings.TrimSpace(current))
	}
	for _, chunk := range chunks {
		count, err := s.count(chunk)
		if err != nil {
			return nil, err
		}
		if count > s.size {
			return nil, fmt.Errorf("chunk exceeds %d-token budget", s.size)
		}
	}
	return chunks, nil
}

func (s *TokenSplitter) splitParts(text string, limit int, separators []string) ([]string, error) {
	count, err := s.count(text)
	if err != nil {
		return nil, err
	}
	if count <= limit {
		return []string{text}, nil
	}
	if len(separators) == 0 || separators[0] == "" {
		ids, _, err := s.codec.Encode(text)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, (len(ids)+limit-1)/limit)
		for start := 0; start < len(ids); start += limit {
			end := min(start+limit, len(ids))
			piece, err := s.codec.Decode(ids[start:end])
			if err != nil {
				return nil, err
			}
			out = append(out, piece)
		}
		return out, nil
	}
	separator := separators[0]
	pieces := strings.SplitAfter(text, separator)
	if len(pieces) == 1 {
		return s.splitParts(text, limit, separators[1:])
	}
	var out []string
	for _, piece := range pieces {
		if piece == "" {
			continue
		}
		children, err := s.splitParts(piece, limit, separators[1:])
		if err != nil {
			return nil, err
		}
		out = append(out, children...)
	}
	return out, nil
}
