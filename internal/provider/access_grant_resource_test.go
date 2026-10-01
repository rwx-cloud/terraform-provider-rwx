package provider

import (
	"testing"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestAccessGrantMatchesPrincipal(t *testing.T) {
	userResource := accessGrantResource{principalType: api.AccessGrantPrincipalTypeUser}
	if !userResource.matchesPrincipal(api.AccessGrantPrincipal{Type: api.AccessGrantPrincipalTypeUser, Email: "User@Example.com"}, "user@example.com") {
		t.Fatal("user email match was case-sensitive")
	}
	if userResource.matchesPrincipal(api.AccessGrantPrincipal{Type: api.AccessGrantPrincipalTypeServiceAccount, Name: "user@example.com"}, "user@example.com") {
		t.Fatal("principal types matched")
	}

	serviceAccountResource := accessGrantResource{principalType: api.AccessGrantPrincipalTypeServiceAccount}
	if serviceAccountResource.matchesPrincipal(api.AccessGrantPrincipal{Type: api.AccessGrantPrincipalTypeServiceAccount, Name: "Deploys"}, "deploys") {
		t.Fatal("service-account names matched with different casing")
	}
}

func TestEquivalentExpiration(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt *string
		previous  types.String
		want      types.String
	}{
		{
			name:     "permanent",
			previous: types.StringValue("2026-12-01T15:00:00Z"),
			want:     types.StringNull(),
		},
		{
			name:      "same instant preserves configuration",
			expiresAt: testStringPointer("2026-12-01T15:00:00.000000Z"),
			previous:  types.StringValue("2026-12-01T10:00:00-05:00"),
			want:      types.StringValue("2026-12-01T10:00:00-05:00"),
		},
		{
			name:      "drift uses API timestamp",
			expiresAt: testStringPointer("2026-12-02T15:00:00.000000Z"),
			previous:  types.StringValue("2026-12-01T15:00:00Z"),
			want:      types.StringValue("2026-12-02T15:00:00.000000Z"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := equivalentExpiration(tt.expiresAt, tt.previous); !got.Equal(tt.want) {
				t.Fatalf("equivalentExpiration() = %s, want %s", got, tt.want)
			}
		})
	}
}

func testStringPointer(value string) *string {
	return &value
}
