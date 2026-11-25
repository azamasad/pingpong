package filechunk

import (
	"strings"
	"unicode/utf8"
)

// SplitParagraphs turns text into chunks at most maxRunes runes each, using paragraph
// and sentence boundaries per V7.6 (D-112).
func SplitParagraphs(text string, maxRunes int) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var paras []string
	for _, block := range strings.Split(text, "\n\n") {
		lines := strings.Split(block, "\n")
		var cleaned []string
		for _, ln := range lines {
			if strings.TrimSpace(ln) != "" {
				cleaned = append(cleaned, ln)
			}
		}
		if len(cleaned) == 0 {
			continue
		}
		paras = append(paras, strings.Join(cleaned, "\n"))
	}
	if len(paras) == 0 {
		return nil
	}
	var out []string
	var cur string
	flush := func() {
		if strings.TrimSpace(cur) == "" {
			cur = ""
			return
		}
		out = append(out, cur)
		cur = ""
	}
	appendPara := func(p string) {
		if cur == "" {
			cur = p
			return
		}
		combined := cur + "\n\n" + p
		if utf8.RuneCountInString(combined) <= maxRunes {
			cur = combined
			return
		}
		flush()
		cur = p
	}
	for _, p := range paras {
		rest := p
		for rest != "" {
			if utf8.RuneCountInString(rest) <= maxRunes {
				appendPara(rest)
				rest = ""
				break
			}
			cut := hardCutRunes(rest, maxRunes)
			sent := lastSentenceEnd(rest[:cut])
			if sent > 0 {
				cut = sent
			}
			piece := rest[:cut]
			appendPara(piece)
			rest = strings.TrimLeft(rest[cut:], " ")
		}
	}
	flush()
	return out
}

func hardCutRunes(s string, maxRunes int) int {
	n := 0
	for i := range s {
		if n == maxRunes {
			return i
		}
		n++
	}
	return len(s)
}

func lastSentenceEnd(s string) int {
	best := -1
	for i := 0; i < len(s); {
		r, w := utf8.DecodeRuneInString(s[i:])
		if r == '\n' {
			best = i + w
		} else if r == '.' || r == '?' || r == '!' {
			j := i + w
			if j < len(s) && s[j] == ' ' {
				best = j + 1
			} else if j >= len(s) {
				best = j
			}
		}
		i += w
	}
	return best
}
