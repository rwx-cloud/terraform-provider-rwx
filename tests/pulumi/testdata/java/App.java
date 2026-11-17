import java.nio.file.Files;
import java.nio.file.Path;

import com.google.gson.JsonParser;
import com.pulumi.Pulumi;
import com.pulumi.core.Output;
import com.pulumi.rwx.RwxFunctions;
import com.pulumi.rwx.Secret;
import com.pulumi.rwx.SecretArgs;
import com.pulumi.rwx.ServiceAccount;
import com.pulumi.rwx.ServiceAccountArgs;
import com.pulumi.rwx.Variable;
import com.pulumi.rwx.VariableArgs;
import com.pulumi.rwx.Vault;
import com.pulumi.rwx.VaultArgs;
import com.pulumi.rwx.inputs.GetServiceAccountArgs;
import com.pulumi.rwx.inputs.GetServiceAccountPlainArgs;

public class App {
    public static void main(String[] args) throws Exception {
        var settings = JsonParser.parseString(Files.readString(Path.of("scenario.json"))).getAsJsonObject();
        Pulumi.run(ctx -> {
            var name = settings.get("name").getAsString();
            var vault = new Vault("vault", VaultArgs.builder().name(name).build());

            switch (settings.get("scenario").getAsString()) {
                case "lifecycle":
                    var secret = SecretArgs.builder().name("shared").secretValue(settings.get("secret").getAsString());
                    var value = settings.has("updated") && settings.get("updated").getAsBoolean() ? "after" : "before";
                    var variable = VariableArgs.builder().name("shared").value(value);
                    if (settings.get("useVaultId").getAsBoolean()) {
                        secret.vaultId(vault.id());
                        variable.vaultId(vault.id());
                    } else {
                        secret.vault(vault.name());
                        variable.vault(vault.name());
                    }
                    new Secret("secret", secret.build());
                    new Variable("variable", variable.build());
                    var account = new ServiceAccount("account", ServiceAccountArgs.builder()
                        .displayName(name).description(value).build());
                    ctx.export("ordinary", account.id().apply(id -> Output.of(RwxFunctions.getServiceAccountPlain(
                        GetServiceAccountPlainArgs.builder().id(id).build()).thenApply(found -> found.displayName()))));
                    ctx.export("deferred", RwxFunctions.getServiceAccount(GetServiceAccountArgs.builder()
                        .id(account.id()).build()).applyValue(found -> found.displayName()));
                    break;
                case "invalid-selector":
                    var invalid = VariableArgs.builder().name("invalid").value("invalid");
                    if (settings.get("both").getAsBoolean()) {
                        invalid.vaultId(vault.id()).vault(vault.name());
                    }
                    new Variable("invalid", invalid.build());
                    break;
                default:
                    throw new IllegalArgumentException("Unknown compatibility scenario");
            }
        });
    }
}
