package filechunk

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func runeLen(s string) int { return utf8.RuneCountInString(s) }

func TestChunkParagraphBoundaries(t *testing.T) {
	text := "Para one.\n\nPara two.\n\nPara three."
	chunks := SplitParagraphs(text, 6000)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if !strings.Contains(chunks[0], "Para one.") || !strings.Contains(chunks[0], "Para three.") {
		t.Fatal(chunks[0])
	}
}

func TestChunkNeverExceedsLimit(t *testing.T) {
	long := strings.Repeat("word ", 5000)
	chunks := SplitParagraphs(long, 500)
	for i, c := range chunks {
		if runeLen(c) > 500 {
			t.Fatalf("chunk %d len %d", i, runeLen(c))
		}
	}
}

func TestChunkSentenceCut(t *testing.T) {
	sent1 := strings.Repeat("a", 100) + ". "
	sent2 := strings.Repeat("b", 100) + ". "
	text := sent1 + sent2 + strings.Repeat("c", 200)
	chunks := SplitParagraphs(text, 120)
	for _, c := range chunks {
		if runeLen(c) > 120 {
			t.Fatalf("over limit: %d", runeLen(c))
		}
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
}

func TestChunkMultibyteRuneBoundary(t *testing.T) {
	text := strings.Repeat("é", 50)
	chunks := SplitParagraphs(text, 10)
	for _, c := range chunks {
		if !utf8.ValidString(c) {
			t.Fatal("invalid utf8 in chunk")
		}
		if runeLen(c) > 10 {
			t.Fatalf("chunk too long: %d", runeLen(c))
		}
	}
}

func TestChunkDropsWhitespaceOnly(t *testing.T) {
	chunks := SplitParagraphs("   \n\n   \n\nreal", 6000)
	if len(chunks) != 1 || chunks[0] != "real" {
		t.Fatalf("%+v", chunks)
	}
}

func TestChunkOrderPreserved(t *testing.T) {
	parts := make([]string, 20)
	for i := range parts {
		parts[i] = "section-" + string(rune('a'+i))
	}
	text := strings.Join(parts, "\n\n")
	chunks := SplitParagraphs(text, 40)
	var joined string
	for i, c := range chunks {
		if i > 0 {
			joined += "|"
		}
		joined += c
	}
	for _, p := range parts {
		if !strings.Contains(joined, p) {
			t.Fatalf("missing %q in %q", p, joined)
		}
	}
	idx := -1
	for _, p := range parts {
		i := strings.Index(joined, p)
		if i < idx {
			t.Fatal("order broken")
		}
		idx = i
	}
}
