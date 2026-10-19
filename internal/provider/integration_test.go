package provider

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"
)

func integrationClient(t *testing.T) api.Client {
	t.Helper()
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("set TF_ACC=1 to run live integration tests")
	}
	host := os.Getenv("RWX_HOST")
	if host == "" {
		host = "cloud.rwx.com"
	}
	client, err := api.NewClient(api.Config{
		Host:        host,
		AccessToken: os.Getenv("RWX_ACCESS_TOKEN"),
		Version:     "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodGet, "/api/auth/whoami", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var identity struct {
		OrganizationSlug string `json:"organization_slug"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&identity); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || identity.OrganizationSlug != "rwx-integration-testing" {
		t.Fatal("live tests require credentials for rwx-integration-testing")
	}
	return client
}

func integrationName() string {
	return "tf-integration-" + rand.Text()
}

func integrationVault(t *testing.T) api.Vault {
	t.Helper()
	client := integrationClient(t)
	vault, err := client.CreateVault(api.Vault{Name: integrationName(), RequiredApprovals: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.DeleteVault(vault.ID); err != nil && !errors.Is(err, api.ErrNotFound) {
			t.Errorf("cleanup vault %s: %v", vault.ID, err)
		}
	})
	return vault
}

func integrationAccount(t *testing.T, client api.Client) api.ServiceAccount {
	t.Helper()
	account, err := client.CreateServiceAccount(api.ServiceAccount{DisplayName: integrationName()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.DeleteServiceAccount(account.ID); err != nil && !errors.Is(err, api.ErrNotFound) {
			t.Errorf("cleanup service account %s: %v", account.ID, err)
		}
	})
	return account
}

func integrationError(t *testing.T, err error, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatal(fmt.Errorf("got %v, want %w", err, want))
	}
}
