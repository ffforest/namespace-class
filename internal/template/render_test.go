package template

import "testing"

func TestRenderStringReplacesKnownValues(t *testing.T) {
	got, err := RenderString("{{ .Namespace.Name }}-binding", map[string]string{
		".Namespace.Name": "web-portal",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got != "web-portal-binding" {
		t.Fatalf("unexpected render: %q", got)
	}
}

func TestRenderStringRejectsUnknownValues(t *testing.T) {
	if _, err := RenderString("{{ .Namespace.Missing }}", map[string]string{}); err == nil {
		t.Fatal("expected unknown value to fail")
	}
}
