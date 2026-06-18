package localchecks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type localIntegrationManifest struct {
	SchemaVersion int      `json:"schema_version"`
	Kind          string   `json:"kind"`
	Version       string   `json:"version"`
	Status        string   `json:"status"`
	Owner         string   `json:"owner"`
	Permissions   []string `json:"permissions"`
	Rollback      string   `json:"rollback"`
}

func installedIntegrations(dirs []string) []map[string]any {
	manifests := loadIntegrationManifests(dirs)
	out := make([]map[string]any, 0, len(manifests))
	for _, manifest := range manifests {
		out = append(out, map[string]any{
			"kind":        manifest.Kind,
			"version":     manifest.Version,
			"status":      manifest.Status,
			"owner":       manifest.Owner,
			"permissions": manifest.Permissions,
			"rollback":    manifest.Rollback,
		})
	}
	return out
}

func loadIntegrationManifests(dirs []string) []localIntegrationManifest {
	seen := map[string]bool{}
	out := []localIntegrationManifest{}
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
		if err != nil {
			continue
		}
		sort.Strings(matches)
		for _, path := range matches {
			raw, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var manifest localIntegrationManifest
			if json.Unmarshal(raw, &manifest) != nil || !validIntegrationManifest(manifest) {
				continue
			}
			manifest.Kind = normalizeKind(manifest.Kind)
			manifest.Version = strings.TrimSpace(manifest.Version)
			manifest.Status = strings.ToLower(strings.TrimSpace(manifest.Status))
			key := manifest.Kind + ":" + manifest.Version
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, manifest)
		}
	}
	return out
}

func validIntegrationManifest(manifest localIntegrationManifest) bool {
	kind := normalizeKind(manifest.Kind)
	if kind == "" {
		return false
	}
	if _, ok := integrationCatalog[kind]; !ok {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(manifest.Status))
	if status != "official" && status != "beta" && status != "experimental" {
		return false
	}
	for _, permission := range manifest.Permissions {
		p := strings.ToLower(strings.TrimSpace(permission))
		if strings.Contains(p, "shell") || strings.Contains(p, "exec") || strings.Contains(p, "command") {
			return false
		}
	}
	manifest.Kind = kind
	return strings.TrimSpace(manifest.Version) != ""
}
