provider "apicurio" {
  endpoint = "http://localhost:3080"
}

# Example: set an artifact-level compatibility rule
resource "apicurio_rule" "artifact_compatibility" {
  scope       = "artifact"
  group_id    = "example.group"
  artifact_id = "example-artifact"

  rule_type = "COMPATIBILITY"
  config    = "BACKWARD"
}
