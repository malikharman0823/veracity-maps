package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanLegacyConfigsPreservesSourceAndReportsMalformedFiles(t *testing.T) {
	root := t.TempDir()
	validPath := filepath.Join(root, "sess_valid", "saved_configs.json")
	malformedPath := filepath.Join(root, "sess_malformed", "saved_configs.json")
	if err := os.MkdirAll(filepath.Dir(validPath), 0o755); err != nil {
		t.Fatalf("create valid session directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(malformedPath), 0o755); err != nil {
		t.Fatalf("create malformed session directory: %v", err)
	}
	validJSON := `[{"id":"cfg_1","name":"Campus","primary_dataset_id":"ds_a","members":["ds_a","ds_b"],"configuration":{"active_view":"map"}}]`
	if err := os.WriteFile(validPath, []byte(validJSON), 0o644); err != nil {
		t.Fatalf("write valid legacy config: %v", err)
	}
	if err := os.WriteFile(malformedPath, []byte(`[{`), 0o644); err != nil {
		t.Fatalf("write malformed legacy config: %v", err)
	}

	candidates, skips, err := scanLegacyConfigs(root)
	if err != nil {
		t.Fatalf("scanLegacyConfigs returned error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(candidates))
	}
	if candidates[0].SessionID != "sess_valid" || candidates[0].Config.ID != "cfg_1" {
		t.Fatalf("candidate = %#v, want sess_valid/cfg_1", candidates[0])
	}
	if len(skips) != 1 || skips[0].SessionID != "sess_malformed" || skips[0].Reason != "malformed_json" {
		t.Fatalf("skips = %#v, want malformed_json for sess_malformed", skips)
	}
	if got, err := os.ReadFile(validPath); err != nil || string(got) != validJSON {
		t.Fatalf("valid source changed: bytes=%q err=%v", got, err)
	}
}

func TestResolveLegacyOwnerRequiresOneUnambiguousOwner(t *testing.T) {
	tests := []struct {
		name       string
		assetIDs   []string
		owners     map[string]string
		wantOwner  string
		wantReason string
	}{
		{
			name:      "same owner",
			assetIDs:  []string{"ds_a", "ds_b"},
			owners:    map[string]string{"ds_a": "user_a", "ds_b": "user_a"},
			wantOwner: "user_a",
		},
		{
			name:       "mixed owners",
			assetIDs:   []string{"ds_a", "ds_b"},
			owners:     map[string]string{"ds_a": "user_a", "ds_b": "user_b"},
			wantReason: "mixed_owners",
		},
		{
			name:       "missing asset",
			assetIDs:   []string{"ds_a", "ds_missing"},
			owners:     map[string]string{"ds_a": "user_a"},
			wantReason: "missing_asset",
		},
		{
			name:       "anonymous asset",
			assetIDs:   []string{"ds_a"},
			owners:     map[string]string{"ds_a": ""},
			wantReason: "anonymous_owner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, reason := resolveLegacyOwner(tt.assetIDs, tt.owners)
			if owner != tt.wantOwner || reason != tt.wantReason {
				t.Fatalf("resolveLegacyOwner = (%q, %q), want (%q, %q)", owner, reason, tt.wantOwner, tt.wantReason)
			}
		})
	}
}
