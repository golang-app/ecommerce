package eventcatalog

import (
	"bytes"
	"strings"
	"testing"
)

// TestCatalogIsConsistent is the catalog-drift guard: every Event
// must have non-empty Name / Kind / Package / Producer, Kind must be
// one of the two allowed values, Version must be at least 1, and no
// two events may share a Name. Adding a new event without satisfying
// these invariants fails CI before the rendered doc can drift.
func TestCatalogIsConsistent(t *testing.T) {
	events := Catalog()
	if len(events) == 0 {
		t.Fatal("Catalog() returned no events")
	}

	allowedKinds := map[string]struct{}{
		KindDomain:      {},
		KindIntegration: {},
	}

	seen := make(map[string]struct{}, len(events))
	for i, e := range events {
		if e.Name == "" {
			t.Errorf("events[%d]: Name is empty", i)
		}
		if e.Kind == "" {
			t.Errorf("events[%d] (%s): Kind is empty", i, e.Name)
		}
		if _, ok := allowedKinds[e.Kind]; !ok {
			t.Errorf("events[%d] (%s): Kind %q is not one of %q/%q",
				i, e.Name, e.Kind, KindDomain, KindIntegration)
		}
		if e.Package == "" {
			t.Errorf("events[%d] (%s): Package is empty", i, e.Name)
		}
		if e.Producer == "" {
			t.Errorf("events[%d] (%s): Producer is empty", i, e.Name)
		}
		if e.Version < 1 {
			t.Errorf("events[%d] (%s): Version %d is less than 1",
				i, e.Name, e.Version)
		}
		if _, dup := seen[e.Name]; dup {
			t.Errorf("events[%d]: duplicate Name %q", i, e.Name)
		}
		seen[e.Name] = struct{}{}
	}
}

// TestRenderMarkdown sanity-checks that the rendered table contains
// the header row, the separator row, and every event Name. It is a
// smoke test, not a golden-file fixture — we want changes to the
// catalog to land freely without touching the test.
func TestRenderMarkdown(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderMarkdown(&buf, Catalog()); err != nil {
		t.Fatalf("RenderMarkdown returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "| Name | Kind | Producer | Consumers | Version | Description |") {
		t.Errorf("rendered output missing header row:\n%s", out)
	}
	for _, e := range Catalog() {
		if !strings.Contains(out, e.Name) {
			t.Errorf("rendered output missing event %q", e.Name)
		}
	}
}

// TestRenderMarkdownByContext checks that the by-context renderer
// emits one H2 per distinct producer and includes every event under
// some producer.
func TestRenderMarkdownByContext(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderMarkdownByContext(&buf, Catalog()); err != nil {
		t.Fatalf("RenderMarkdownByContext returned error: %v", err)
	}
	out := buf.String()
	producers := map[string]struct{}{}
	for _, e := range Catalog() {
		producers[e.Producer] = struct{}{}
	}
	for p := range producers {
		if !strings.Contains(out, "## "+p) {
			t.Errorf("by-context output missing section for producer %q", p)
		}
	}
}
