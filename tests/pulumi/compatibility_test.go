package pulumi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"
)

type engine struct {
	t        *testing.T
	project  string
	evidence string
	env      []string
	secrets  []string
	counter  int
	language string
	adapter  string
}

func writeFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}

func (e *engine) command(private, success bool, args ...string) string {
	e.t.Helper()
	e.counter++
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir, cmd.Env = e.project, e.env
	output, err := cmd.CombinedOutput()
	if !private {
		redacted := string(output)
		for _, value := range e.secrets {
			redacted = strings.ReplaceAll(redacted, value, "[REDACTED]")
		}
		writeFile(e.t, filepath.Join(e.evidence, fmt.Sprintf("%03d-%s.log", e.counter, filepath.Base(args[0]))), redacted)
	}
	e.t.Logf("%s: success=%t", strings.Join(args, " "), err == nil)
	if (err == nil) != success {
		e.t.Fatalf("command %d: unexpected exit: %v; see artifacts", e.counter, err)
	}
	return string(output)
}

func (e *engine) run(args ...string) string { return e.command(false, true, args...) }

func (e *engine) configure(settings map[string]any) {
	e.t.Helper()
	data, err := json.Marshal(settings)
	if err != nil {
		e.t.Fatal(err)
	}
	writeFile(e.t, filepath.Join(e.project, "scenario.json"), string(data))
}

func (e *engine) up() { e.run("pulumi", "up", "--yes", "--skip-preview", "--non-interactive") }

type resource struct {
	URN     string         `json:"urn"`
	Type    string         `json:"type"`
	ID      string         `json:"id"`
	Outputs map[string]any `json:"outputs"`
}

func (e *engine) checkpoint(private bool) []resource {
	e.t.Helper()
	args := []string{"pulumi", "stack", "export"}
	if private {
		args = append(args, "--show-secrets")
	}
	var state struct {
		Deployment struct {
			Resources []resource `json:"resources"`
		} `json:"deployment"`
	}
	if err := json.Unmarshal([]byte(e.command(private, true, args...)), &state); err != nil {
		e.t.Fatal(err)
	}
	return state.Deployment.Resources
}

func findResource(t *testing.T, rows []resource, kind string) resource {
	t.Helper()
	for _, row := range rows {
		if strings.HasSuffix(row.Type, ":"+kind) {
			return row
		}
	}
	t.Fatalf("missing %s resource", kind)
	return resource{}
}

func secretOutput(t *testing.T, row resource, field string) string {
	t.Helper()
	value := row.Outputs[field]
	if wrapped, ok := value.(map[string]any); ok {
		plaintext, ok := wrapped["plaintext"].(string)
		if !ok || json.Unmarshal([]byte(plaintext), &value) != nil {
			t.Fatalf("invalid secret output %s", field)
		}
	}
	text, ok := value.(string)
	if !ok || text == "" {
		t.Fatalf("missing secret output %s", field)
	}
	return text
}

func equal(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestPulumiCompatibility(t *testing.T) {
	if os.Getenv("PULUMI_E2E") != "1" {
		t.Skip("set PULUMI_E2E=1 and RWX_PROVIDER_BINARY to run live Pulumi compatibility tests")
	}
	host := os.Getenv("RWX_HOST")
	if host == "" {
		host = "cloud.rwx.com"
	}
	client, err := api.NewClient(api.Config{Host: host, AccessToken: os.Getenv("RWX_ACCESS_TOKEN"), Version: "test", Pulumi: true})
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
	var identity struct {
		OrganizationSlug string `json:"organization_slug"`
	}
	err = json.NewDecoder(resp.Body).Decode(&identity)
	_ = resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK || identity.OrganizationSlug != "rwx-integration-testing" {
		t.Fatal("live tests require credentials for rwx-integration-testing")
	}
	adapter, err := exec.Command("mise", "config", "get", "--file", "../../mise.toml", "vars.pulumi_terraform_adapter").CombinedOutput()
	if err != nil {
		t.Fatalf("read adapter version: %v: %s", err, adapter)
	}
	work := t.TempDir()
	language := os.Getenv("PULUMI_TEST_LANGUAGE")
	if language == "" {
		language = "typescript"
	}
	switch language {
	case "typescript", "python", "go", "dotnet":
	default:
		t.Fatalf("unsupported Pulumi language: %s", language)
	}
	t.Logf("Pulumi language: %s; adapter: %s", language, strings.TrimSpace(string(adapter)))
	evidence := filepath.Join(os.Getenv("RWX_ARTIFACTS"), "pulumi")
	if err := os.MkdirAll(evidence, 0700); err != nil {
		t.Fatal(err)
	}
	name, secret, passphrase := "pulumi-integration-"+rand.Text(), rand.Text(), rand.Text()
	e := &engine{t: t, project: work, evidence: evidence, secrets: []string{secret, passphrase, os.Getenv("RWX_ACCESS_TOKEN")},
		language: language, adapter: strings.TrimSpace(string(adapter))}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "RWX_") && !strings.HasPrefix(entry, "PULUMI_") && !strings.HasPrefix(entry, "TF_") && !strings.HasPrefix(entry, "GH_") && !strings.HasPrefix(entry, "GITHUB_") {
			e.env = append(e.env, entry)
		}
	}
	e.env = append(e.env, "PULUMI_HOME="+filepath.Join(work, "home"), "PULUMI_BACKEND_URL=file://"+filepath.Join(work, "backend"),
		"PULUMI_CONFIG_PASSPHRASE="+passphrase, "PULUMI_SKIP_UPDATE_CHECK=true")
	if err := os.Mkdir(filepath.Join(work, "backend"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"mise.toml", ".tool-versions"} {
		data, err := os.ReadFile(filepath.Join("../..", file))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(evidence, file), string(data))
	}
	programDir := filepath.Join("testdata", language)
	files, err := os.ReadDir(programDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(programDir, file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(work, file.Name()), string(data))
	}
	if language == "python" {
		e.run("python", "-m", "venv", "venv")
	}
	e.run("pulumi", "package", "add", "terraform-provider@"+strings.TrimSpace(string(adapter)), os.Getenv("RWX_PROVIDER_BINARY"))
	switch language {
	case "typescript":
		e.run("npm", "install", "--no-audit", "--no-fund")
	case "python":
		e.run("venv/bin/pip", "install", "-r", "requirements.txt")
	case "go":
		e.run("go", "mod", "tidy")
	case "dotnet":
		e.run("dotnet", "restore")
	}
	e.run("pulumi", "login", "file://"+filepath.Join(work, "backend"), "--non-interactive")
	e.run("pulumi", "stack", "init", "compatibility", "--non-interactive")
	e.env = append(e.env, "RWX_HOST="+host, "RWX_ACCESS_TOKEN="+os.Getenv("RWX_ACCESS_TOKEN"))
	settings := map[string]any{"scenario": "lifecycle", "name": name, "secret": secret, "useVaultId": true}
	inventory := func() map[string]int {
		counts := map[string]int{"vaults": 0, "accounts": 0}
		if _, err := client.FindVaultByName(name); err == nil {
			counts["vaults"]++
		} else if !errors.Is(err, api.ErrNotFound) {
			t.Errorf("inventory vault: %v", err)
		}
		accounts, err := client.ListServiceAccounts(name)
		if err != nil {
			t.Errorf("inventory service account: %v", err)
		}
		for _, account := range accounts {
			if account.DisplayName == name {
				counts["accounts"]++
			}
		}
		return counts
	}
	t.Cleanup(func() {
		defer func() {
			remaining := inventory()
			for kind, count := range remaining {
				if count != 0 {
					t.Errorf("Pulumi destroy left %d test %s", count, kind)
				}
			}
			accounts, err := client.ListServiceAccounts(name)
			if err != nil {
				t.Errorf("fallback account inventory: %v", err)
			}
			for _, account := range accounts {
				if account.DisplayName == name {
					if err := client.DeleteServiceAccount(account.ID); err != nil {
						t.Errorf("fallback account cleanup: %v", err)
					}
				}
			}
			if vault, err := client.FindVaultByName(name); err == nil {
				if err := client.DeleteVault(vault.ID); err != nil {
					t.Errorf("fallback vault cleanup: %v", err)
				}
			}
			final := inventory()
			data, err := json.MarshalIndent(map[string]any{"name": name, "after_destroy": remaining, "after_cleanup": final}, "", "  ")
			if err != nil {
				t.Error(err)
			}
			writeFile(t, filepath.Join(evidence, "cleanup.json"), string(data))
		}()
		settings["scenario"] = "lifecycle"
		e.configure(settings)
		e.run("pulumi", "destroy", "--yes", "--refresh=false", "--non-interactive")
	})
	e.configure(settings)
	e.run("pulumi", "preview", "--non-interactive")
	equal(t, inventory(), map[string]int{"vaults": 0, "accounts": 0})
	settings["useVaultId"] = false
	e.configure(settings)
	e.up()
	initial := e.checkpoint(false)
	vaultID := findResource(t, initial, "Vault").ID
	canonicalID := "vault_id/" + vaultID + "/shared"
	for _, kind := range []string{"Secret", "Variable"} {
		equal(t, findResource(t, initial, kind).ID, canonicalID)
	}
	selector := api.VaultSelector{ID: vaultID}
	metadata, err := client.GetSecretMetadataInVault(selector, api.Secret{Name: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	plaintext := e.checkpoint(true)
	token := secretOutput(t, findResource(t, plaintext, "ServiceAccount"), "token")
	e.secrets = append(e.secrets, token)
	state, _ := json.Marshal(initial)
	if strings.Contains(string(state), secret) || strings.Contains(string(state), token) {
		t.Fatal("plaintext sensitive output in encrypted checkpoint")
	}
	e.run("pulumi", "refresh", "--yes", "--non-interactive")
	e.run("pulumi", "preview", "--expect-no-changes", "--non-interactive")
	settings["useVaultId"], settings["updated"] = true, true
	e.configure(settings)
	e.up()
	variable, err := client.GetVariableInVault(selector, api.Variable{Name: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	equal(t, variable.Value, "after")
	unchanged, err := client.GetSecretMetadataInVault(selector, api.Secret{Name: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	equal(t, unchanged.Version, metadata.Version)
	plaintext = e.checkpoint(true)
	if secretOutput(t, findResource(t, plaintext, "Secret"), "secretValue") != secret || secretOutput(t, findResource(t, plaintext, "ServiceAccount"), "token") != token {
		t.Fatal("sensitive output did not retain its exact value")
	}
	var outputs map[string]string
	if err := json.Unmarshal([]byte(e.run("pulumi", "stack", "output", "--json")), &outputs); err != nil {
		t.Fatal(err)
	}
	wantOutputs := map[string]string{"deferred": name, "ordinary": name}
	equal(t, outputs, wantOutputs)
	if _, err := client.SetVariableInVault(selector, api.Variable{Name: "shared", Value: "drift"}); err != nil {
		t.Fatal(err)
	}
	e.run("pulumi", "refresh", "--yes", "--non-interactive")
	e.up()
	variable, err = client.GetVariableInVault(selector, api.Variable{Name: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	equal(t, variable.Value, "after")
	secretResource := findResource(t, initial, "Secret")
	e.run("pulumi", "state", "delete", secretResource.URN, "--yes", "--non-interactive")
	e.run("pulumi", "import", secretResource.Type, "secret", canonicalID, "--yes", "--non-interactive", "--generate-code=false", "--protect=false")
	imported, err := client.GetSecretMetadataInVault(selector, api.Secret{Name: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	equal(t, imported.Version, metadata.Version)
	e.up()
	updated, err := client.GetSecretMetadataInVault(selector, api.Secret{Name: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	equal(t, updated.Version, metadata.Version+1)
	e.run("pulumi", "refresh", "--yes", "--non-interactive")
	equal(t, findResource(t, e.checkpoint(false), "Secret").ID, canonicalID)
	missing := e.command(false, false, "pulumi", "import", "rwx:index/vault:Vault", "missing", "00000000-0000-4000-8000-000000000000", "--yes", "--non-interactive", "--generate-code=false", "--protect=false")
	if !strings.Contains(missing, "Cannot import non-existent remote object") {
		t.Fatal("missing Vault import failed for an unexpected reason")
	}
	for _, row := range e.checkpoint(false) {
		if strings.HasSuffix(row.URN, "::missing") {
			t.Fatal("missing import checkpointed a resource")
		}
	}
	for _, both := range []bool{true, false} {
		settings["scenario"], settings["both"] = "invalid-selector", both
		e.configure(settings)
		diagnostic := e.command(false, false, "pulumi", "preview", "--non-interactive")
		if !strings.Contains(diagnostic, "Invalid Attribute Combination") {
			t.Fatal("invalid selector preview failed for an unexpected reason")
		}
	}
}
