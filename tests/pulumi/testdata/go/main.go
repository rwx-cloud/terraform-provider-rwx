package main

import (
	"encoding/json"
	"fmt"
	"os"

	rwx "github.com/pulumi/pulumi-terraform-provider/sdks/go/rwx/rwx"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		data, err := os.ReadFile("scenario.json")
		if err != nil {
			return err
		}
		var settings struct {
			Scenario   string `json:"scenario"`
			Name       string `json:"name"`
			Secret     string `json:"secret"`
			UseVaultID bool   `json:"useVaultId"`
			Updated    bool   `json:"updated"`
			Both       bool   `json:"both"`
		}
		if err := json.Unmarshal(data, &settings); err != nil {
			return err
		}
		vault, err := rwx.NewVault(ctx, "vault", &rwx.VaultArgs{Name: pulumi.String(settings.Name)})
		if err != nil {
			return err
		}
		var vaultID, vaultName pulumi.StringPtrInput
		if settings.UseVaultID {
			vaultID = vault.ID().ToStringOutput()
		} else {
			vaultName = vault.Name
		}
		switch settings.Scenario {
		case "lifecycle":
			_, err = rwx.NewSecret(ctx, "secret", &rwx.SecretArgs{
				VaultId: vaultID, Vault: vaultName, Name: pulumi.String("shared"), SecretValue: pulumi.String(settings.Secret),
			})
			if err != nil {
				return err
			}
			value := "before"
			if settings.Updated {
				value = "after"
			}
			_, err = rwx.NewVariable(ctx, "variable", &rwx.VariableArgs{
				VaultId: vaultID, Vault: vaultName, Name: pulumi.String("shared"), Value: pulumi.String(value),
			})
			if err != nil {
				return err
			}
			account, err := rwx.NewServiceAccount(ctx, "account", &rwx.ServiceAccountArgs{
				DisplayName: pulumi.String(settings.Name), Description: pulumi.String(value),
			})
			if err != nil {
				return err
			}
			ordinary := account.ID().ApplyT(func(id pulumi.ID) (string, error) {
				text := string(id)
				found, err := rwx.LookupServiceAccount(ctx, &rwx.LookupServiceAccountArgs{Id: &text})
				if err != nil {
					return "", err
				}
				return found.DisplayName, nil
			})
			ctx.Export("ordinary", ordinary)
			ctx.Export("deferred", rwx.LookupServiceAccountOutput(ctx, rwx.LookupServiceAccountOutputArgs{
				Id: account.ID().ToStringOutput(),
			}).DisplayName())
		case "invalid-selector":
			args := &rwx.VariableArgs{Name: pulumi.String("invalid"), Value: pulumi.String("invalid")}
			if settings.Both {
				args.VaultId, args.Vault = vault.ID().ToStringOutput(), vault.Name
			}
			_, err = rwx.NewVariable(ctx, "invalid", args)
			return err
		default:
			return fmt.Errorf("unknown compatibility scenario: %s", settings.Scenario)
		}
		return nil
	})
}
