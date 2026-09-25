package rag

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	defaultOffTopicAnswer = "Maaf, saya tidak bisa memberikan informasi yang anda mau."
	defaultNotFoundAnswer = "Maaf, Menu yang anda cari tidak ditemukan."
)

// Guardrails are executable policy loaded from Markdown files maintained by
// the application owner. Prose is included in the system prompt, while the
// fenced JSON rules are enforced by Go before and after model calls.
type Guardrails struct {
	Prompt         string
	OffTopicAnswer string
	NotFoundAnswer string
	MaxQueryChars  int
	MaxAnswerChars int
	MaxDistance    float64
	InputRules     []guardrailRule
	OutputRules    []guardrailRule
}

type guardrailRule struct {
	Pattern  string `json:"pattern"`
	Response string `json:"response,omitempty"`
	re       *regexp.Regexp
}

type guardrailFile struct {
	OffTopicAnswer string          `json:"off_topic_answer"`
	NotFoundAnswer string          `json:"not_found_answer"`
	MaxQueryChars  int             `json:"max_query_chars"`
	MaxAnswerChars int             `json:"max_answer_chars"`
	MaxDistance    *float64        `json:"max_distance"`
	InputRules     []guardrailRule `json:"input_rules"`
	OutputRules    []guardrailRule `json:"output_rules"`
}

func DefaultGuardrails() Guardrails {
	return Guardrails{OffTopicAnswer: defaultOffTopicAnswer, NotFoundAnswer: defaultNotFoundAnswer,
		MaxQueryChars: 500, MaxAnswerChars: 1500, MaxDistance: 0.85}
}

// LoadGuardrails reads all .md files recursively in lexical order. Invalid
// rules fail startup/request instead of silently weakening the policy.
func LoadGuardrails(dir string) (Guardrails, error) {
	rules := DefaultGuardrails()
	entries := []string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".md") {
			entries = append(entries, path)
		}
		return nil
	})
	if err != nil {
		return rules, err
	}
	if len(entries) == 0 {
		return rules, fmt.Errorf("no Markdown guardrails found in %s", dir)
	}
	sort.Strings(entries)
	for _, path := range entries {
		content, err := os.ReadFile(path)
		if err != nil {
			return rules, err
		}
		prose, blocks, err := guardrailBlocks(string(content))
		if err != nil {
			return rules, fmt.Errorf("%s: %w", path, err)
		}
		if strings.TrimSpace(prose) != "" {
			rules.Prompt += "\n" + strings.TrimSpace(prose)
		}
		for _, block := range blocks {
			var parsed guardrailFile
			decoder := json.NewDecoder(strings.NewReader(block))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&parsed); err != nil {
				return rules, fmt.Errorf("%s: invalid guardrail JSON: %w", path, err)
			}
			var trailing any
			if err := decoder.Decode(&trailing); err != io.EOF {
				return rules, fmt.Errorf("%s: guardrail block must contain one JSON object", path)
			}
			if parsed.OffTopicAnswer != "" {
				rules.OffTopicAnswer = parsed.OffTopicAnswer
			}
			if parsed.NotFoundAnswer != "" {
				rules.NotFoundAnswer = parsed.NotFoundAnswer
			}
			if parsed.MaxQueryChars < 0 || parsed.MaxAnswerChars < 0 {
				return rules, fmt.Errorf("%s: limits cannot be negative", path)
			}
			if parsed.MaxQueryChars > 0 {
				rules.MaxQueryChars = parsed.MaxQueryChars
			}
			if parsed.MaxAnswerChars > 0 {
				rules.MaxAnswerChars = parsed.MaxAnswerChars
			}
			if parsed.MaxDistance != nil {
				if *parsed.MaxDistance <= 0 || *parsed.MaxDistance > 2 {
					return rules, fmt.Errorf("%s: max_distance must be within (0, 2]", path)
				}
				rules.MaxDistance = *parsed.MaxDistance
			}
			for _, group := range []struct {
				source []guardrailRule
				target *[]guardrailRule
			}{{parsed.InputRules, &rules.InputRules}, {parsed.OutputRules, &rules.OutputRules}} {
				for _, rule := range group.source {
					if rule.Pattern == "" {
						return rules, fmt.Errorf("%s: empty regex pattern", path)
					}
					compiled, err := regexp.Compile(rule.Pattern)
					if err != nil {
						return rules, fmt.Errorf("%s: regex %q: %w", path, rule.Pattern, err)
					}
					rule.re = compiled
					*group.target = append(*group.target, rule)
				}
			}
		}
	}
	rules.Prompt = strings.TrimSpace(rules.Prompt)
	return rules, nil
}

func guardrailBlocks(markdown string) (string, []string, error) {
	lines := strings.Split(markdown, "\n")
	var prose, blocks []string
	var block []string
	inBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock && trimmed == "```guardrail" {
			inBlock = true
			block = nil
			continue
		}
		if inBlock && trimmed == "```" {
			blocks = append(blocks, strings.Join(block, "\n"))
			inBlock = false
			continue
		}
		if inBlock {
			block = append(block, line)
		} else {
			prose = append(prose, line)
		}
	}
	if inBlock {
		return "", nil, fmt.Errorf("unclosed guardrail code block")
	}
	return strings.Join(prose, "\n"), blocks, nil
}

func (g Guardrails) CheckInput(query string) (string, string) {
	if g.MaxQueryChars > 0 && len([]rune(query)) > g.MaxQueryChars {
		return fmt.Sprintf("Pertanyaan terlalu panjang (maksimal %d karakter).", g.MaxQueryChars), "query_too_long"
	}
	for _, rule := range g.InputRules {
		if rule.re.MatchString(query) {
			if rule.Response != "" {
				return rule.Response, "input_rule"
			}
			return g.OffTopicAnswer, "input_rule"
		}
	}
	return "", ""
}

func (g Guardrails) CheckRetrieval(chunks []Chunk) bool {
	if len(chunks) == 0 {
		return false
	}
	best := chunks[0].Distance
	for _, chunk := range chunks[1:] {
		if chunk.Distance < best {
			best = chunk.Distance
		}
	}
	return best <= g.MaxDistance
}

func (g Guardrails) CheckOutput(answer string) (string, string) {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return g.NotFoundAnswer, "empty_answer"
	}
	for _, rule := range g.OutputRules {
		if rule.re.MatchString(answer) {
			if rule.Response != "" {
				return rule.Response, "output_rule"
			}
			return g.NotFoundAnswer, "output_rule"
		}
	}
	if g.MaxAnswerChars > 0 && len([]rune(answer)) > g.MaxAnswerChars {
		return g.NotFoundAnswer, "answer_too_long"
	}
	return answer, ""
}
