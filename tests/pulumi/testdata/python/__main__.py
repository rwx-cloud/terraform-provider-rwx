import json

import pulumi
import pulumi_rwx as rwx

with open("scenario.json") as scenario:
    settings = json.load(scenario)

vault = rwx.Vault("vault", name=settings["name"])

if settings["scenario"] == "lifecycle":
    selector = {"vault_id": vault.id} if settings["useVaultId"] else {"vault": vault.name}
    rwx.Secret("secret", **selector, name="shared", secret_value=settings["secret"])
    value = "after" if settings.get("updated") else "before"
    rwx.Variable("variable", **selector, name="shared", value=value)
    account = rwx.ServiceAccount("account", display_name=settings["name"], description=value)
    pulumi.export("ordinary", account.id.apply(lambda id: rwx.get_service_account(id=id).display_name))
    pulumi.export("deferred", rwx.get_service_account_output(id=account.id).display_name)
elif settings["scenario"] == "invalid-selector":
    selector = {"vault": vault.name, "vault_id": vault.id} if settings["both"] else {}
    rwx.Variable("invalid", **selector, name="invalid", value="invalid")
else:
    raise ValueError(f"Unknown compatibility scenario: {settings['scenario']}")
