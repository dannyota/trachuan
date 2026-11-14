# Plan and apply OCI infrastructure

`deploy/oci/` describes the OCI resources of the production host: compute,
volumes, network, buckets, IAM users, groups and policies, the vault and its
key, logging, alarms and email. DNS is not here; it is managed with the `cf` CLI
([deployment](../design/deployment.md#domain-and-dns)). Customer secret keys,
SMTP credentials and vault secret contents are never managed here, so state
holds no secret values.

## Credentials

- **Provider:** the OCI CLI profile in `~/.oci/config`. Plans use a profile that
  can read the tenancy; only the coordinator applies.
- **State:** Object Storage bucket `trachuan-tfstate` (private, versioned)
  through its S3-compatible endpoint. The IAM user `trachuan-tfstate` holds one
  customer secret key, kept in `~/.config/trachuan/tfstate.env` (mode 0600) as
  `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`. Its policy reads that bucket
  and manages its objects only. Source the file in a subshell; never print it.
- **Variables:** copy `terraform.tfvars.example` to `terraform.tfvars` (ignored
  by Git) and fill in the tenancy and compartment OCIDs and the alarm email.

## Plan

```sh
cd deploy/oci
(set -a; . ~/.config/trachuan/tfstate.env; set +a
 tofu init -input=false
 tofu plan -input=false)
```

A plan on an unchanged tenancy reports no changes. Any change in a plan must be
explained before it is applied.

## Apply

Only the coordinator applies, from a reviewed pull request with green CI on its
commit, using the same subshell with `tofu apply`. The backend has no lock, so
state-writing operations (`apply`, `import`, `state` commands) run one at a time
from the coordinator's session only; plans may run in parallel because they
write nothing. The instance, volumes, reserved IP, buckets, vault and key carry
`prevent_destroy`; a plan that would replace one of them stops at plan time and
needs a reviewed change of approach, never a forced apply.

State and saved plans contain service-user emails and creator tags. Keep any
local copy (`tofu state pull`, a saved plan, or the `errored.tfstate` OpenTofu
writes when the backend rejects a write) in private scratch storage, never in
the repository; `.gitignore` excludes state and plan files. To recover from a
failed write, push the local copy with `tofu state push` once the backend
answers; the bucket keeps earlier versions for rollback.

## Import a resource created outside OpenTofu

Add an `import` block with the resource address and the import identifier the
provider documents for that resource type in a local file that Git ignores
(`imports.tf`). Most types take the OCID; buckets take
`n/{namespace}/b/{bucket}`, logs `logGroupId/{logGroupId}/logId/{logId}` and
vault keys `managementEndpoint/{endpoint}/keys/{keyId}`. Write the matching
resource configuration and run a plan until it shows only the import and no
changes. The coordinator applies the import; then delete the local file.
