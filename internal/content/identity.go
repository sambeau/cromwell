package content

import (
	"errors"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// A document's identity lives in its front matter, as two keys, `id:` and
// `revision:` (SPEC-015 SD-8). These functions read and write exactly those
// two lines, as text, so comments, key order, quoting, line endings and the
// body are kept byte for byte. They never re-serialise the YAML.

// ErrUnclosedFrontMatter is a file that opens a front matter block and never
// closes it: rewriting it would guess at where the block ends.
var ErrUnclosedFrontMatter = errors.New("the front matter is never closed: the file starts with --- but has no closing --- line")

const bom = "\uFEFF"

// frontMatterSpan locates the front matter: the newline style, the index just
// after the opening fence line, and the index of the closing fence line's
// start. ok is false when the file has no front matter.
func frontMatterSpan(raw string) (nl string, bodyStart, closeStart int, ok bool, err error) {
	start := 0
	if strings.HasPrefix(raw, bom) {
		start = len(bom)
	}
	rest := raw[start:]
	switch {
	case strings.HasPrefix(rest, "---\r\n"):
		nl = "\r\n"
	case strings.HasPrefix(rest, "---\n"):
		nl = "\n"
	default:
		return "", 0, 0, false, nil
	}
	bodyStart = start + 3 + len(nl)
	pos := bodyStart
	for pos <= len(raw) {
		end := strings.Index(raw[pos:], "\n")
		line := raw[pos:]
		if end >= 0 {
			line = raw[pos : pos+end]
		}
		if strings.TrimRight(line, "\r") == "---" {
			return nl, bodyStart, pos, true, nil
		}
		if end < 0 {
			break
		}
		pos += end + 1
	}
	return "", 0, 0, false, ErrUnclosedFrontMatter
}

// identityKey reports whether a front-matter line sets a top-level key.
func identityKey(line, key string) bool {
	if !strings.HasPrefix(line, key) {
		return false
	}
	rest := strings.TrimLeft(line[len(key):], " \t")
	return strings.HasPrefix(rest, ":")
}

// ReadIdentity returns the `id:` and `revision:` a file's front matter
// carries. ok is false when either is missing or malformed.
func ReadIdentity(raw string) (id string, revision int, ok bool) {
	_, bodyStart, closeStart, found, err := frontMatterSpan(raw)
	if err != nil || !found {
		return "", 0, false
	}
	fm := strings.ReplaceAll(raw[bodyStart:closeStart], "\r\n", "\n")
	var meta struct {
		ID       any `yaml:"id"`
		Revision any `yaml:"revision"`
	}
	if yaml.Unmarshal([]byte(fm), &meta) != nil {
		return "", 0, false
	}
	idStr, isStr := meta.ID.(string)
	if !isStr || strings.TrimSpace(idStr) == "" {
		return "", 0, false
	}
	switch r := meta.Revision.(type) {
	case int:
		revision = r
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(r))
		if err != nil {
			return "", 0, false
		}
		revision = n
	default:
		return "", 0, false
	}
	if revision < 1 {
		return "", 0, false
	}
	return strings.TrimSpace(idStr), revision, true
}

// ReadDeclaredID returns a front-matter `id:` value whether or not a revision
// accompanies it: adopt checks what a file already claims to be (FR-5.4).
func ReadDeclaredID(raw string) string {
	_, bodyStart, closeStart, found, err := frontMatterSpan(raw)
	if err != nil || !found {
		return ""
	}
	fm := strings.ReplaceAll(raw[bodyStart:closeStart], "\r\n", "\n")
	var meta struct {
		ID any `yaml:"id"`
	}
	if yaml.Unmarshal([]byte(fm), &meta) != nil {
		return ""
	}
	if s, ok := meta.ID.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// SetIdentity writes `id:` and `revision:` into a file's front matter: an
// existing top-level line is replaced; a missing one is inserted directly after
// the opening fence; a file with no front matter gets a block of just the two,
// a blank line, then the file as it was.
func SetIdentity(raw, id string, revision int) (string, error) {
	idLine := "id: " + id
	revLine := "revision: " + strconv.Itoa(revision)
	nl, bodyStart, closeStart, found, err := frontMatterSpan(raw)
	if err != nil {
		return "", err
	}
	if !found {
		prefix := ""
		if strings.HasPrefix(raw, bom) {
			prefix, raw = bom, raw[len(bom):]
		}
		nl = "\n"
		if i := strings.Index(raw, "\n"); i > 0 && raw[i-1] == '\r' {
			nl = "\r\n"
		}
		return prefix + "---" + nl + idLine + nl + revLine + nl + "---" + nl + nl + raw, nil
	}

	lines := strings.SplitAfter(raw[bodyStart:closeStart], "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	haveID, haveRev := false, false
	for i, l := range lines {
		ending := l[len(strings.TrimRight(l, "\r\n")):]
		switch {
		case identityKey(l, "id") && !haveID:
			lines[i], haveID = idLine+ending, true
		case identityKey(l, "revision") && !haveRev:
			lines[i], haveRev = revLine+ending, true
		}
	}
	var insert string
	if !haveID {
		insert += idLine + nl
	}
	if !haveRev {
		insert += revLine + nl
	}
	return raw[:bodyStart] + insert + strings.Join(lines, "") + raw[closeStart:], nil
}

// StripIdentity removes the top-level `id:` and `revision:` lines (Detach,
// SD-13). If that empties the front matter, the block goes too, with the
// blank line SetIdentity put after it, so an identity added to a file with no
// front matter is removed without trace.
func StripIdentity(raw string) (string, error) {
	nl, bodyStart, closeStart, found, err := frontMatterSpan(raw)
	if err != nil || !found {
		return raw, err
	}
	lines := strings.SplitAfter(raw[bodyStart:closeStart], "\n")
	var kept []string
	removed := false
	for _, l := range lines {
		if identityKey(l, "id") || identityKey(l, "revision") {
			removed = true
			continue
		}
		kept = append(kept, l)
	}
	if !removed {
		return raw, nil
	}
	fm := strings.Join(kept, "")
	if strings.TrimSpace(fm) == "" {
		start := 0
		if strings.HasPrefix(raw, bom) {
			start = len(bom)
		}
		after := closeStart + 3 + len(nl)
		if after > len(raw) {
			after = len(raw)
		}
		rest := strings.TrimPrefix(raw[after:], nl)
		return raw[:start] + rest, nil
	}
	return raw[:bodyStart] + fm + raw[closeStart:], nil
}
