import { readFileSync } from "node:fs";
import * as rwx from "@pulumi/rwx";

const settings = JSON.parse(readFileSync("scenario.json", "utf8"));

let ordinary;
let deferred;

const vault = new rwx.Vault("vault", { name: settings.name });

switch (settings.scenario) {
    case "lifecycle": {
        const selector = settings.useVaultId ? { vaultId: vault.id } : { vault: vault.name };
        new rwx.Secret("secret", {
            ...selector,
            name: "shared",
            secretValue: settings.secret,
        });
        new rwx.Variable("variable", {
            ...selector,
            name: "shared",
            value: settings.updated ? "after" : "before",
        });
        const account = new rwx.ServiceAccount("account", {
            displayName: settings.name,
            description: settings.updated ? "after" : "before",
        });
        ordinary = account.id.apply(id => rwx.getServiceAccount({ id })).apply(a => a.displayName);
        deferred = rwx.getServiceAccountOutput({ id: account.id }).displayName;
        break;
    }
    case "invalid-selector":
        new rwx.Variable("invalid", {
            ...(settings.both ? { vault: vault.name, vaultId: vault.id } : {}),
            name: "invalid",
            value: "invalid",
        });
        break;
    default:
        throw new Error(`Unknown compatibility scenario: ${settings.scenario}`);
}

export { ordinary, deferred };
