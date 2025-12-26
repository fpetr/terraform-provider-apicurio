# Copyright (c) HashiCorp, Inc.

terraform {
  required_providers {
    apicurio = {
      source  = "fpetr/apicurio"
      version = ">= 0.0.0"
    }
  }
}

variable "apicurio_endpoint" {
  type = string
}

variable "schemas_root" {
  type        = string
  description = "Path to the repo 'schemas' directory"
  default     = "schemas"
}

provider "apicurio" {
  endpoint = var.apicurio_endpoint

  # Optional auth
  # token       = var.apicurio_token
  # auth_header = "Authorization: Bearer ${var.apicurio_token}"
}

# Expected repo structure:
# - schemas/<group>/schemas.yml
# - schemas/<group>/<ArtifactId>.json
#
# Example schemas.yml:
# apicurio_deploy_group: com.example.common.v1
# schemas:
#   - artifactId: ErrorCommonMessage
#     version: v1
#     labels:
#       - com.example.control.pravidla.otk.v1.public.error
#       - com.example.control.pravidla.ai.v1.public.error

locals {
  group_files = fileset(var.schemas_root, "*/schemas.yml")

  group_cfgs = {
    for relpath in local.group_files :
    dirname(relpath) => yamldecode(file("${var.schemas_root}/${relpath}"))
  }

  artifacts = merge([
    for group_dir, cfg in local.group_cfgs : {
      for s in cfg.schemas :
      "${cfg.apicurio_deploy_group}/${s.artifactId}" => {
        group_id      = cfg.apicurio_deploy_group
        artifact_id   = s.artifactId
        version       = try(s.version, null)
        labels        = try(s.labels, [])
        content_file  = "${var.schemas_root}/${group_dir}/${s.artifactId}.json"
        artifact_type = "AVRO"
      }
    }
  ]...)
}

resource "apicurio_artifact" "schema" {
  for_each = local.artifacts

  group_id      = each.value.group_id
  artifact_id   = each.value.artifact_id
  artifact_type = each.value.artifact_type

  content_file = each.value.content_file
  version      = each.value.version
  labels       = each.value.labels

  # Keep UI consistent:
  name = each.value.artifact_id
}
