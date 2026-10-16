# Choose either the vault-name or vault-ID form for an existing secret.
terraform import rwx_secret.example example/my-secret

terraform import rwx_secret.example vault_id/00000000-0000-0000-0000-000000000000/my-secret
