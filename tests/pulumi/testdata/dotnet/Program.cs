using System.Text.Json;
using Pulumi;
using Rwx = Pulumi.Rwx;

return await Deployment.RunAsync(() =>
{
    var settings = JsonDocument.Parse(File.ReadAllText("scenario.json")).RootElement;
    var name = settings.GetProperty("name").GetString()!;
    var vault = new Rwx.Vault("vault", new() { Name = name });

    switch (settings.GetProperty("scenario").GetString())
    {
        case "lifecycle":
            var useVaultId = settings.GetProperty("useVaultId").GetBoolean();
            Input<string>? vaultId = null;
            Input<string>? vaultName = null;
            if (useVaultId)
            {
                vaultId = vault.Id;
            }
            else
            {
                vaultName = vault.Name;
            }
            var value = settings.TryGetProperty("updated", out var updated) && updated.GetBoolean() ? "after" : "before";
            _ = new Rwx.Secret("secret", new()
            {
                VaultId = vaultId, Vault = vaultName, Name = "shared", SecretValue = settings.GetProperty("secret").GetString()!,
            });
            _ = new Rwx.Variable("variable", new()
            {
                VaultId = vaultId, Vault = vaultName, Name = "shared", Value = value,
            });
            var account = new Rwx.ServiceAccount("account", new() { DisplayName = name, Description = value });
            return new Dictionary<string, object?>
            {
                ["ordinary"] = account.Id.Apply(async id => (await Rwx.GetServiceAccount.InvokeAsync(new() { Id = id })).DisplayName),
                ["deferred"] = Rwx.GetServiceAccount.Invoke(new() { Id = account.Id }).Apply(found => found.DisplayName),
            };
        case "invalid-selector":
            var args = new Rwx.VariableArgs { Name = "invalid", Value = "invalid" };
            if (settings.GetProperty("both").GetBoolean())
            {
                args.VaultId = vault.Id;
                args.Vault = vault.Name;
            }
            _ = new Rwx.Variable("invalid", args);
            return new Dictionary<string, object?>();
        default:
            throw new InvalidOperationException("Unknown compatibility scenario");
    }
});
