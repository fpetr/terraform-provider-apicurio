# Copyright (c) HashiCorp, Inc.

# Import a GLOBAL rule.
# Format: global/<rule_type>
terraform import apicurio_rule.example "global/COMPATIBILITY"

# Import an ARTIFACT rule.
# Format: <group_id>/<artifact_id>/<rule_type>
terraform import apicurio_rule.example "com.example.common.v1/ErrorCommonMessage/COMPATIBILITY"
