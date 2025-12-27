# Terraform Provider Apicurio

Terraform provider for managing Apicurio Registry artifacts (group/artifactId/custom version string/labels) with automatic v3/v2 detection.

This is designed for organizations that:
- organize artifacts by `group_id` and `artifact_id`
- use version strings like `v1`, `v2` (not numeric Schema Registry versions)
- optional `name` for display name

## Requirements

- Terraform >= 1.0
- Go >= 1.24

## Architecture overview

- Provider implementation: `internal/provider/provider.go`
- Main entrypoint: `cmd/terraform-provider-apicurio/main.go`
- Registry abstraction: `internal/client/client.go` (`RegistryClient` interface)
- v2 implementation: `internal/client/v2.go` (Core API v2)
- v3 implementation: `internal/client/v3.go` (uses v3 metadata endpoints when available; falls back to v2 where needed)

The provider can be pinned to an Apicurio API flavor via `api_version` (`v2` or `v3`).

If `api_version` is not set, the provider will best-effort probe (preferring `v3`):
- tries `GET {endpoint}/apis/registry/v3/system/info`
- else tries `GET {endpoint}/apis/registry/v2/system/info`
- defaults to `v3` if probing fails (best-effort)

## Provider configuration

```hcl
provider "apicurio" {
	endpoint = "http://localhost:3080"
	# api_version = "v3" # optional; "v2" or "v3" (default "v3")

	# Optional auth (choose one)
	# token       = var.apicurio_token
	# auth_header = "Authorization: Bearer ${var.apicurio_token}"

	# basic_auth = {
	#   username = var.apicurio_username
	#   password = var.apicurio_password
	# }

	# oidc = {
	#   token_url      = "https://keycloak.example/realms/myrealm/protocol/openid-connect/token"
	#   client_id      = var.keycloak_client_id
	#   client_secret  = var.keycloak_client_secret
	#   scopes         = ["openid"]
	#   extra_params   = { audience = "apicurio" }
	#   auth_style     = "auto" # or "in_header" / "in_params"
	# }

	# Optional TLS
	# insecure_skip_verify = true
	# ca_bundle_path       = "/path/to/ca-bundle.pem"
}
```

## Resource: `apicurio_artifact`

Manages an artifact and its versions.

```hcl
resource "apicurio_artifact" "example" {
	group_id      = "com.example.common.v1"
	artifact_id   = "ErrorCommonMessage"
	artifact_type = "AVRO" # default

	# Exactly one:
	content      = file("schemas/com.example.common.v1/ErrorCommonMessage.json")
	# content_file = "schemas/com.example.common.v1/ErrorCommonMessage.json"

	version = "v1"

	# Optional display name
	# name        = "ErrorCommonMessage"
	description = "Shared error envelope"

	labels = [
		"com.example.control.pravidla.otk.v1.public.error",
		"com.example.control.pravidla.ai.v1.public.error",
	]

	hard_delete = false
}
```

Import format:

```bash
terraform import apicurio_artifact.example "<group_id>/<artifact_id>"
```

## Resource: `apicurio_rule`

Manages a rule either globally or for a specific artifact.

```hcl
resource "apicurio_rule" "artifact_compatibility" {
	scope       = "artifact"
	group_id    = "com.example.common.v1"
	artifact_id = "ErrorCommonMessage"

	rule_type = "COMPATIBILITY"
	config    = "BACKWARD"
}
```

Import formats:

```bash
terraform import apicurio_rule.artifact_compatibility "<group_id>/<artifact_id>/<rule_type>"
terraform import apicurio_rule.some_global_rule "global/<rule_type>"
```

## API call mapping

The provider prefers v3 endpoints when available and falls back to v2.

- Create artifact:
	- `POST /apis/registry/v2/groups/{group}/artifacts`
	- headers:
		- `X-Registry-ArtifactId: {artifact_id}`
		- `X-Registry-ArtifactType: {artifact_type}`
		- `X-Registry-Version: {version}` (if set)
	- body: raw content
- Update/read metadata (preferred v3):
	- `PUT /apis/registry/v3/groups/{group}/artifacts/{artifact}`
	- `GET /apis/registry/v3/groups/{group}/artifacts/{artifact}`
	- version metadata (when `version` is set):
		- `PUT /apis/registry/v3/groups/{group}/artifacts/{artifact}/versions/{version}`
	- if v3 endpoints return 404, the provider falls back to v2 `/meta` endpoints.
- Read latest content (best-effort for drift hash):
	- `GET /apis/registry/v2/groups/{group}/artifacts/{artifact}`
- Create new version:
	- `POST /apis/registry/v2/groups/{group}/artifacts/{artifact}/versions`
	- header `X-Registry-Version: {version}` if provided
- Check if a version exists:
	- `GET /apis/registry/v2/groups/{group}/artifacts/{artifact}/versions/{version}/meta`
- Delete a specific version (used when `allow_overwrite_version=true`):
	- `DELETE /apis/registry/v2/groups/{group}/artifacts/{artifact}/versions/{version}`
- Delete artifact:
	- `DELETE /apis/registry/v2/groups/{group}/artifacts/{artifact}?hardDelete=true` (if `hard_delete`)

## v3/v2 strategy

If `api_version` is unset, the provider probes `/apis/registry/v3/system/info` first and prefers v3.
If a v3 endpoint returns 404, the provider falls back to the corresponding v2 endpoint (best-effort).

## Build and run locally

### Local Apicurio (PostgreSQL + OIDC)

This repo includes a Docker Compose setup that runs Apicurio Registry backed by PostgreSQL and protected by OIDC (Keycloak).

- Compose file: `compose.yml`
- Env template: `.env.apicurio.example`
- UI: `http://localhost:8888`
- API: `http://localhost:8080` (v3 base is `/apis/registry/v3`)

```bash
cp .env.apicurio.example .env
# Edit OIDC_AUTH_SERVER_URL to your realm issuer URL (typically https://login.example.com/realms/<realm>)
docker compose --env-file .env -f compose.yml up -d
```

Notes:

- `OIDC_AUTH_SERVER_URL` should be the realm issuer URL (Keycloak typically requires the `/realms/<realm>` suffix).
- Configure Keycloak clients and set `OIDC_API_CLIENT_ID` and `OIDC_UI_CLIENT_ID`.
- `OIDC_REDIRECT_URL` must match the UI client redirect URI (defaults to `http://localhost:8888`).

Build the provider binary:

```bash
go build -o terraform-provider-apicurio ./cmd/terraform-provider-apicurio
```

Run acceptance tests against a local Apicurio at `http://localhost:3080`:

```bash
export APICURIO_ENDPOINT="http://localhost:3080"
# export APICURIO_TOKEN="..."   # optional
# export APICURIO_AUTH_HEADER="Authorization: Bearer ..."  # optional
go test ./... -run TestAccApicurioArtifact_basic
```

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.24

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

```shell
go install
```

## Adding Dependencies

This provider uses [Go modules](https://github.com/golang/go/wiki/Modules).
Please see the Go documentation for the most up to date information about using Go modules.

To add a new dependency `github.com/author/dependency` to your Terraform provider:

```shell
go get github.com/author/dependency
go mod tidy
```

Then commit the changes to `go.mod` and `go.sum`.

## Using the provider

Fill this in for each provider

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `make generate`.

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```shell
make testacc
```
