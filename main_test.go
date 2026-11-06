package main

import (
	"strings"
	"testing"
)

func TestCatalogUniqueAndSafe(t *testing.T) {
	a := newApp()
	if len(a.catalog) < 150 {
		t.Fatalf("expected extensive catalog, got %d", len(a.catalog))
	}
	seen := map[string]bool{}
	for _, tw := range a.catalog {
		if seen[tw.ID] {
			t.Fatalf("duplicate ID %s", tw.ID)
		}
		seen[tw.ID] = true
		if tw.Kind == "registry" && len(tw.Changes) == 0 {
			t.Fatalf("registry tweak missing changes: %s", tw.ID)
		}
		if tw.Kind == "appx" && tw.Package == "" {
			t.Fatalf("package tweak missing package: %s", tw.ID)
		}
	}
}
func TestFortniteINI(t *testing.T) {
	src := "[ScalabilityGroups]\r\nsg.TextureQuality=3\r\nsg.ShadowQuality=3\r\n[Other]\r\nRemain=ok\r\n"
	out := updateINI(src, map[string]map[string]string{"[ScalabilityGroups]": {"sg.TextureQuality": "0", "sg.ShadowQuality": "0", "sg.FoliageQuality": "0"}})
	if !strings.Contains(out, "sg.TextureQuality=0\r\n") || !strings.Contains(out, "sg.FoliageQuality=0\r\n") || !strings.Contains(out, "Remain=ok") {
		t.Fatalf("INI write incorrect: %q", out)
	}
}
func TestFortniteINISectionAdd(t *testing.T) {
	out := updateINI("[Other]\nKeep=1", map[string]map[string]string{"[ScalabilityGroups]": {"sg.TextureQuality": "0"}})
	if !strings.Contains(out, "[ScalabilityGroups]\nsg.TextureQuality=0") {
		t.Fatalf("INI section missing: %q", out)
	}
}
