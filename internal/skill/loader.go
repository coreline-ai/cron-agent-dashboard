package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const MaxSkillMDBytes = 128 * 1024

var ErrInvalidSkill = errors.New("invalid skill")

type Document struct {
	Name        string
	Description string
	Triggers    []string
	Body        string
	Raw         string
	Hash        string
}

func LoadFile(path string) (Document, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "." || clean == "" {
		return Document{}, fmt.Errorf("%w: path is required", ErrInvalidSkill)
	}
	info, err := os.Stat(clean)
	if err != nil {
		return Document{}, err
	}
	if info.IsDir() {
		clean = filepath.Join(clean, "SKILL.md")
		info, err = os.Stat(clean)
		if err != nil {
			return Document{}, err
		}
	}
	if info.Size() > MaxSkillMDBytes {
		return Document{}, fmt.Errorf("%w: SKILL.md is too large", ErrInvalidSkill)
	}
	b, err := os.ReadFile(clean)
	if err != nil {
		return Document{}, err
	}
	return Parse(string(b))
}

func Parse(raw string) (Document, error) {
	if len([]byte(raw)) > MaxSkillMDBytes {
		return Document{}, fmt.Errorf("%w: SKILL.md is too large", ErrInvalidSkill)
	}
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return Document{}, fmt.Errorf("%w: missing frontmatter", ErrInvalidSkill)
	}
	rest := strings.TrimPrefix(normalized, "---\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return Document{}, fmt.Errorf("%w: missing frontmatter end", ErrInvalidSkill)
	}
	frontmatter := rest[:end]
	body := strings.TrimPrefix(rest[end:], "\n---")
	body = strings.TrimPrefix(body, "\n")

	doc := Document{Body: strings.TrimSpace(body), Raw: raw, Hash: Hash(raw)}
	if err := parseFrontmatter(frontmatter, &doc); err != nil {
		return Document{}, err
	}
	doc.Name = normalizeName(doc.Name)
	doc.Description = strings.TrimSpace(doc.Description)
	doc.Triggers = normalizeTriggers(doc.Triggers)
	if doc.Name == "" || doc.Description == "" {
		return Document{}, fmt.Errorf("%w: name and description are required", ErrInvalidSkill)
	}
	return doc, nil
}

func Hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func parseFrontmatter(fm string, doc *Document) error {
	lines := strings.Split(fm, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return fmt.Errorf("%w: invalid frontmatter line %q", ErrInvalidSkill, line)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "name":
			doc.Name = unquote(value)
		case "description":
			doc.Description = unquote(value)
		case "triggers", "trigger":
			if value != "" {
				doc.Triggers = append(doc.Triggers, parseInlineList(value)...)
				continue
			}
			for i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if !strings.HasPrefix(next, "-") {
					break
				}
				i++
				doc.Triggers = append(doc.Triggers, unquote(strings.TrimSpace(strings.TrimPrefix(next, "-"))))
			}
		}
	}
	return nil
}

func parseInlineList(value string) []string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, unquote(strings.TrimSpace(part)))
	}
	return out
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func normalizeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, " ", "-")
	return name
}

func normalizeTriggers(xs []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		key := strings.ToLower(x)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, x)
	}
	return out
}
