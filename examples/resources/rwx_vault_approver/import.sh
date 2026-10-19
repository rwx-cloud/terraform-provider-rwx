rwx vaults approvers list --vault 00000000-0000-0000-0000-000000000000

# Replace the second UUID with the first-column ID from the row matching the user's email.
terraform import rwx_vault_approver.example 00000000-0000-0000-0000-000000000000/11111111-1111-1111-1111-111111111111
