package api

import (
	"fmt"
	"strconv"
	"strings"
)

// This file provides a deliberately small YAML subset parser. It exists only
// so the OpenAPI conformance validator can read the protected contract at
// contracts/api/openapi.yaml without adding a third-party dependency. The
// contract uses a consistent, simple structure: block mappings and sequences,
// flow mappings and arrays, single-quoted and plain scalars, and one folded
// block scalar (info.description). Nothing beyond that subset is required.

// yamlLine is one non-blank, comment-stripped input line.
type yamlLine struct {
	indent int
	text   string
	num    int
}

type yamlParser struct {
	lines []yamlLine
	pos   int
}

func parseYAML(data []byte) (any, error) {
	raw := strings.Split(string(data), "\n")
	lines := make([]yamlLine, 0, len(raw))
	for i, ln := range raw {
		ln = strings.ReplaceAll(ln, "\t", "  ")
		trimmed := strings.TrimLeft(ln, " ")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lines = append(lines, yamlLine{
			indent: len(ln) - len(trimmed),
			text:   trimmed,
			num:    i + 1,
		})
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("yaml: empty document")
	}
	p := &yamlParser{lines: lines}
	return p.parseBlock(0)
}

func (p *yamlParser) peek() (yamlLine, bool) {
	if p.pos >= len(p.lines) {
		return yamlLine{}, false
	}
	return p.lines[p.pos], true
}

func (p *yamlParser) parseBlock(indent int) (any, error) {
	ln, ok := p.peek()
	if !ok || ln.indent < indent {
		return nil, nil
	}
	if ln.indent != indent {
		return nil, p.err(ln, "unexpected indentation %d (expected %d)", ln.indent, indent)
	}
	if strings.HasPrefix(ln.text, "-") {
		return p.parseSequence(indent)
	}
	return p.parseMapping(indent)
}

func (p *yamlParser) err(ln yamlLine, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("yaml: line %d: %s", ln.num, msg)
}

func (p *yamlParser) parseMapping(indent int) (map[string]any, error) {
	out := map[string]any{}
	for {
		ln, ok := p.peek()
		if !ok {
			break
		}
		if ln.indent < indent {
			break
		}
		if ln.indent != indent {
			return nil, p.err(ln, "unexpected indentation %d in mapping", ln.indent)
		}
		if strings.HasPrefix(ln.text, "-") {
			break
		}
		p.pos++

		key, value, hasChildren, isEntry, err := splitMappingEntry(ln.text)
		if err != nil {
			return nil, p.err(ln, "%v", err)
		}
		if !isEntry {
			return nil, p.err(ln, "expected 'key: value' entry, got %q", ln.text)
		}
		switch {
		case hasChildren:
			childIndent, ok := p.nextIndent(ln.indent)
			if !ok {
				out[key] = nil
				continue
			}
			child, err := p.parseBlock(childIndent)
			if err != nil {
				return nil, err
			}
			out[key] = child
		case value == ">-" || value == ">" || value == "|":
			out[key] = p.parseBlockScalar(ln.indent)
		default:
			v, err := parseValue(value)
			if err != nil {
				return nil, p.err(ln, "%v", err)
			}
			out[key] = v
		}
	}
	return out, nil
}

// parseSequence parses a block sequence of items starting at the given indent.
func (p *yamlParser) parseSequence(indent int) ([]any, error) {
	var out []any
	for {
		ln, ok := p.peek()
		if !ok {
			break
		}
		if ln.indent < indent {
			break
		}
		if ln.indent != indent {
			return nil, p.err(ln, "unexpected indentation %d in sequence", ln.indent)
		}
		if !strings.HasPrefix(ln.text, "-") {
			break
		}
		p.pos++

		rest := strings.TrimSpace(strings.TrimPrefix(ln.text, "-"))
		if rest == "" {
			childIndent, ok := p.nextIndent(ln.indent)
			if !ok {
				out = append(out, nil)
				continue
			}
			child, err := p.parseBlock(childIndent)
			if err != nil {
				return nil, err
			}
			out = append(out, child)
			continue
		}
		if strings.HasPrefix(rest, "{") || strings.HasPrefix(rest, "[") {
			v, err := parseValue(rest)
			if err != nil {
				return nil, p.err(ln, "%v", err)
			}
			out = append(out, v)
			continue
		}
		key, value, hasChildren, isEntry, err := splitMappingEntry(rest)
		if err != nil {
			return nil, p.err(ln, "%v", err)
		}
		if isEntry {
			item := map[string]any{}
			if hasChildren {
				childIndent, ok := p.nextIndent(ln.indent)
				if !ok {
					item[key] = nil
				} else {
					child, err := p.parseBlock(childIndent)
					if err != nil {
						return nil, err
					}
					item[key] = child
				}
			} else {
				v, err := parseValue(value)
				if err != nil {
					return nil, p.err(ln, "%v", err)
				}
				item[key] = v
			}
			out = append(out, item)
			continue
		}
		v, err := parseValue(rest)
		if err != nil {
			return nil, p.err(ln, "%v", err)
		}
		out = append(out, v)
	}
	return out, nil
}

// parseBlockScalar consumes the folded/literal block that follows a scalar
// directive and returns its joined text.
func (p *yamlParser) parseBlockScalar(parentIndent int) string {
	var parts []string
	for {
		ln, ok := p.peek()
		if !ok || ln.indent <= parentIndent {
			break
		}
		p.pos++
		parts = append(parts, strings.TrimSpace(ln.text))
	}
	return strings.Join(parts, " ")
}

func (p *yamlParser) nextIndent(current int) (int, bool) {
	ln, ok := p.peek()
	if !ok {
		return 0, false
	}
	if ln.indent <= current {
		return 0, false
	}
	return ln.indent, true
}

// splitMappingEntry splits "key: value" (or a bare "key:"). Quoted keys such
// as '200' are unquoted.
func splitMappingEntry(text string) (key, value string, hasChildren, isEntry bool, err error) {
	if len(text) > 0 && (text[0] == '\'' || text[0] == '"') {
		quote := text[0]
		for i := 1; i < len(text); i++ {
			if text[i] == quote {
				if i+1 < len(text) && text[i+1] == ':' {
					key = unquote(text[:i+1])
					value = strings.TrimSpace(text[i+2:])
					return key, value, value == "", true, nil
				}
				break
			}
		}
	}
	idx := strings.Index(text, ":")
	if idx < 0 {
		return "", "", false, false, nil
	}
	key = strings.TrimSpace(text[:idx])
	value = strings.TrimSpace(text[idx+1:])
	if key == "" {
		return "", "", false, false, fmt.Errorf("empty mapping key")
	}
	return key, value, value == "", true, nil
}

func parseValue(s string) (any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	switch {
	case s == "null" || s == "~":
		return nil, nil
	case s == "true":
		return true, nil
	case s == "false":
		return false, nil
	case strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}"):
		return parseFlow(s[1:len(s)-1], true)
	case strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]"):
		return parseFlow(s[1:len(s)-1], false)
	}
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') {
		return unquote(s), nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, nil
	}
	return s, nil
}

// parseFlow parses an inline flow map or array. isMap selects map semantics.
func parseFlow(inner string, isMap bool) (any, error) {
	parts := splitFlow(inner)
	if isMap {
		out := map[string]any{}
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			key, value, err := splitFlowKV(part)
			if err != nil {
				return nil, err
			}
			v, err := parseValue(value)
			if err != nil {
				return nil, err
			}
			out[key] = v
		}
		return out, nil
	}
	var out []any
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		v, err := parseValue(part)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func splitFlowKV(part string) (string, string, error) {
	for i := 0; i < len(part); i++ {
		c := part[i]
		if c == '\'' || c == '"' {
			quote := c
			for i++; i < len(part); i++ {
				if part[i] == quote {
					break
				}
			}
			continue
		}
		if c == ':' {
			key := strings.TrimSpace(part[:i])
			value := strings.TrimSpace(part[i+1:])
			if key == "" {
				return "", "", fmt.Errorf("empty flow key in %q", part)
			}
			return unquote(key), value, nil
		}
	}
	return "", "", fmt.Errorf("flow mapping entry has no colon: %q", part)
}

// splitFlow splits a flow collection on top-level commas.
func splitFlow(s string) []string {
	var parts []string
	depth := 0
	quote := byte(0)
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// unquote removes a surrounding quote pair from a quoted scalar or key.
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}
