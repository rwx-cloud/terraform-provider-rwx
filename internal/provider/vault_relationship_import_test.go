package provider

import "testing"

func TestVaultRelationshipImportID(t *testing.T) {
	tests := []struct {
		name           string
		value          string
		wantVaultID    string
		wantResourceID string
		wantOK         bool
	}{
		{name: "valid", value: "vault-id/resource-id", wantVaultID: "vault-id", wantResourceID: "resource-id", wantOK: true},
		{name: "missing resource ID", value: "vault-id/", wantOK: false},
		{name: "missing vault ID", value: "/resource-id", wantOK: false},
		{name: "resource ID only", value: "resource-id", wantOK: false},
		{name: "extra component", value: "vault-id/resource-id/extra", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vaultID, resourceID, ok := vaultRelationshipImportID(tt.value)
			if vaultID != tt.wantVaultID || resourceID != tt.wantResourceID || ok != tt.wantOK {
				t.Fatalf("vaultRelationshipImportID(%q) = (%q, %q, %t), want (%q, %q, %t)", tt.value, vaultID, resourceID, ok, tt.wantVaultID, tt.wantResourceID, tt.wantOK)
			}
		})
	}
}
