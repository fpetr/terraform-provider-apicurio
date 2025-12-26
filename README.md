# Terraform Provider Apicurio (Terraform Plugin Framework)

Terraform provider for managing Apicurio Registry artifacts using **Core Registry API v2 semantics** (group/artifactId/custom version string/labels).

This is designed for organizations that:
- organize artifacts by `group_id` and `artifact_id`
- use version strings like `v1`, `v2` (not numeric Schema Registry versions)
- want Apicurio UI to show `<group> / <artifactId>` with `name` defaulting to `artifact_id`

## Requirements

- Terraform >= 1.0
- Go >= 1.24

## Architecture overview

- Provider implementation: `internal/provider/provider.go`
- Main entrypoint: `cmd/terraform-provider-apicurio/main.go`
- Registry abstraction: `internal/client/client.go` (`RegistryClient` interface)
- v2 implementation: `internal/client/v2.go` (Core API v2)
- v3 fallback: `internal/client/v3.go` (currently delegates to v2 semantics; keeps the abstraction layer in place)

The provider can be pinned to an Apicurio API flavor via `api_version` (`v2` or `v3`).

If `api_version` is not set, the provider defaults to `v3` and will best-effort probe:
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

	# Defaults to artifact_id (keeps UI clean)
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

## API call mapping (Core API v2)

The provider uses these endpoints when available under `/apis/registry/v2`:

- Create artifact:
	- `POST /apis/registry/v2/groups/{group}/artifacts`
	- headers:
		- `X-Registry-ArtifactId: {artifact_id}`
		- `X-Registry-ArtifactType: {artifact_type}`
		- `X-Registry-Version: {version}` (if set)
	- body: raw content
- Update metadata:
	- `PUT /apis/registry/v2/groups/{group}/artifacts/{artifact}/meta`
	- body includes `name`, `description`, and `labels`
	- labels are sent as a string map (keys -> "true") with fallbacks to (keys -> null) and a string array (server-dependent)
	- if `version` is set in the Terraform resource, the provider also updates that version metadata:
		- `PUT /apis/registry/v2/groups/{group}/artifacts/{artifact}/versions/{version}/meta`
- Read metadata:
	- `GET /apis/registry/v2/groups/{group}/artifacts/{artifact}/meta`
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

## v3 fallback strategy

On servers that report a v3 system endpoint, the provider currently still performs CRUD using Core API v2 semantics (many Apicurio v3 deployments expose v2 Core API endpoints).

If you encounter a v3-only server where v2 endpoints are truly absent, extend `internal/client/v3.go` to map the `RegistryClient` interface to the correct v3 paths without changing provider/resource code.

## Build and run locally

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
