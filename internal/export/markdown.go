package export

import (
	"fmt"
	"io"
	"strings"
)

// WriteMarkdownSection writes one job as a Markdown section.
func WriteMarkdownSection(w io.Writer, r Row, includeThinking bool) error {
	title := markdownTitle(r)
	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "%s · %s · %s", r.Status, r.Machine, r.Model)
	if r.OutputTokensPerS != "" {
		fmt.Fprintf(&b, " · %s tok/s", r.OutputTokensPerS)
	}
	b.WriteString("\n\n")

	promptFence := FenceFor(r.Prompt)
	b.WriteString("### Prompt\n\n")
	b.WriteString(promptFence)
	b.WriteString("\n")
	b.WriteString(r.Prompt)
	b.WriteString("\n")
	b.WriteString(promptFence)
	b.WriteString("\n\n")

	outFence := FenceFor(r.Output)
	b.WriteString("### Output\n\n")
	b.WriteString(outFence)
	b.WriteString("\n")
	b.WriteString(r.Output)
	b.WriteString("\n")
	b.WriteString(outFence)
	b.WriteString("\n")

	if includeThinking && r.Thinking != "" {
		thinkFence := FenceFor(r.Thinking)
		b.WriteString("\n### Thinking\n\n")
		b.WriteString(thinkFence)
		b.WriteString("\n")
		b.WriteString(r.Thinking)
		b.WriteString("\n")
		b.WriteString(thinkFence)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	_, err := io.Copy(w, strings.NewReader(b.String()))
	return err
}

func markdownTitle(r Row) string {
	if r.Index != "" {
		if r.Stage != "" {
			return fmt.Sprintf("## Item %s (%s)", r.Index, r.Stage)
		}
		return fmt.Sprintf("## Item %s", r.Index)
	}
	if r.Stage != "" {
		return fmt.Sprintf("## Item (%s)", r.Stage)
	}
	return "## Item"
}
