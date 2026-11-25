package templates

import (
	"regexp"
	"sort"
)

var blankRe = regexp.MustCompile(`^[a-z0-9_]+$`)
var nameRe = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

// ValidName reports whether a template name matches the allowed pattern.
func ValidName(name string) bool {
	return nameRe.MatchString(name)
}

// BlanksInOrder returns each blank name once, in order of first appearance in body.
func BlanksInOrder(body string) []string {
	var out []string
	seen := map[string]struct{}{}
	i := 0
	for i < len(body) {
		if i+1 < len(body) && body[i] == '{' && body[i+1] == '{' {
			end := i + 2
			for end < len(body) && body[end] != '}' {
				end++
			}
			if end+1 < len(body) && body[end] == '}' && body[end+1] == '}' {
				name := body[i+2 : end]
				if blankRe.MatchString(name) {
					if _, ok := seen[name]; !ok {
						seen[name] = struct{}{}
						out = append(out, name)
					}
				}
				i = end + 2
				continue
			}
		}
		i++
	}
	return out
}

// Substitute replaces {{name}} blanks in body with vars values. Values are inserted
// verbatim and never scanned again. Returns missing blank names when a value is absent.
func Substitute(body string, vars map[string]string) (string, []string, []string) {
	var missing []string
	used := map[string]bool{}
	var b []byte
	i := 0
	for i < len(body) {
		if i+1 < len(body) && body[i] == '{' && body[i+1] == '{' {
			end := i + 2
			for end < len(body) && body[end] != '}' {
				end++
			}
			if end+1 < len(body) && body[end] == '}' && body[end+1] == '}' {
				name := body[i+2 : end]
				if blankRe.MatchString(name) {
					v, ok := vars[name]
					if !ok || v == "" {
						missing = append(missing, name)
						b = append(b, body[i:end+2]...)
					} else {
						used[name] = true
						b = append(b, v...)
					}
					i = end + 2
					continue
				}
			}
		}
		b = append(b, body[i])
		i++
	}
	var unused []string
	for k := range vars {
		if !used[k] {
			unused = append(unused, k)
		}
	}
	sort.Strings(unused)
	sort.Strings(missing)
	return string(b), missing, unused
}