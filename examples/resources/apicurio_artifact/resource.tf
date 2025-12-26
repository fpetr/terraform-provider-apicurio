# Copyright (c) HashiCorp, Inc.

provider "apicurio" {
  endpoint = "http://localhost:3080"
}

resource "apicurio_artifact" "example" {
  group_id    = "com.example.common.v1"
  artifact_id = "ErrorCommonMessage"

  artifact_type = "AVRO"
  version       = "v1"

  content_file = "schemas/com.example.common.v1/ErrorCommonMessage.json"

  # Keep UI clean: defaults to artifact_id, but explicit is fine too
  name        = "ErrorCommonMessage"
  description = "Shared error envelope"

  labels = [
    "com.example.control.pravidla.otk.v1.public.error",
    "com.example.control.pravidla.ai.v1.public.error",
  ]

  hard_delete = false
}
