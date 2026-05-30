package eventcatalog

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// RenderMarkdown writes the catalog as a single Markdown table sorted
// by Name. The columns are Name, Kind, Producer, Consumers, Version,
// Description — the order chosen so the eye reads "what is it /
// where does it come from / where does it go / what shape / why" in
// one sweep.
func RenderMarkdown(w io.Writer, events []Event) error {
	sorted := append([]Event(nil), events...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	if _, err := fmt.Fprintln(w, "| Name | Kind | Producer | Consumers | Version | Description |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| --- | --- | --- | --- | --- | --- |"); err != nil {
		return err
	}
	for _, e := range sorted {
		if _, err := fmt.Fprintf(w, "| %s | %s | %s | %s | %d | %s |\n",
			e.Name, e.Kind, e.Producer, joinConsumers(e.Consumers), e.Version, escapePipes(e.Description),
		); err != nil {
			return err
		}
	}
	return nil
}

// RenderMarkdownByContext groups the catalog into per-producer
// sections (one H2 per producer) before rendering each section's
// table. Useful for context-map style documentation where the
// producing context is the anchor.
func RenderMarkdownByContext(w io.Writer, events []Event) error {
	byProducer := make(map[string][]Event)
	for _, e := range events {
		byProducer[e.Producer] = append(byProducer[e.Producer], e)
	}

	producers := make([]string, 0, len(byProducer))
	for p := range byProducer {
		producers = append(producers, p)
	}
	sort.Strings(producers)

	for i, p := range producers {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "## %s\n\n", p); err != nil {
			return err
		}
		if err := RenderMarkdown(w, byProducer[p]); err != nil {
			return err
		}
	}
	return nil
}

// joinConsumers renders the consumer slice for the Markdown column.
// Empty slices become a single dash so the table column stays
// non-empty (some Markdown renderers collapse empty cells).
func joinConsumers(c []string) string {
	if len(c) == 0 {
		return "-"
	}
	return escapePipes(strings.Join(c, "; "))
}

// escapePipes escapes the pipe character so descriptions / consumer
// strings can't break the Markdown table structure. We use the
// HTML-ish escape rather than a backslash because GitHub-flavoured
// Markdown is more reliable about rendering the entity.
func escapePipes(s string) string {
	return strings.ReplaceAll(s, "|", "&#124;")
}
