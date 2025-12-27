// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccApicurioDataSourceRule_artifact_exists(t *testing.T) {
	endpoint := os.Getenv("APICURIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("set APICURIO_ENDPOINT to run acceptance tests")
	}

	if os.Getenv("APICURIO_TEST_RULES") != "1" {
		t.Skip("set APICURIO_TEST_RULES=1 to run rule acceptance tests")
	}

	groupID := os.Getenv("APICURIO_TEST_GROUP")
	if groupID == "" {
		groupID = "tf.acc.rules"
	}

	artifactID := fmt.Sprintf("tf-ds-rule-%d", time.Now().UnixNano())
	content := `{"type":"record","name":"DsRuleAccTest","namespace":"tf.acc.rules","fields":[{"name":"message","type":"string"}]}`

	ruleType := os.Getenv("APICURIO_TEST_RULE_TYPE")
	if ruleType == "" {
		ruleType = "COMPATIBILITY"
	}
	ruleConfig := os.Getenv("APICURIO_TEST_RULE_CONFIG1")
	if ruleConfig == "" {
		ruleConfig = "BACKWARD"
	}

	resourceName := "data.apicurio_rule.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccApicurioDataSourceRuleArtifactConfig(endpoint, groupID, artifactID, content, ruleType, ruleConfig),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "scope", "artifact"),
					resource.TestCheckResourceAttr(resourceName, "group_id", groupID),
					resource.TestCheckResourceAttr(resourceName, "artifact_id", artifactID),
					resource.TestCheckResourceAttr(resourceName, "rule_type", ruleType),
					resource.TestCheckResourceAttr(resourceName, "enabled", "true"),
					resource.TestCheckResourceAttr(resourceName, "config", ruleConfig),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
		},
	})
}

func TestAccApicurioDataSourceRule_artifact_missing(t *testing.T) {
	endpoint := os.Getenv("APICURIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("set APICURIO_ENDPOINT to run acceptance tests")
	}

	if os.Getenv("APICURIO_TEST_RULES") != "1" {
		t.Skip("set APICURIO_TEST_RULES=1 to run rule acceptance tests")
	}

	groupID := os.Getenv("APICURIO_TEST_GROUP")
	if groupID == "" {
		groupID = "tf.acc.rules"
	}

	artifactID := fmt.Sprintf("tf-ds-rule-missing-%d", time.Now().UnixNano())

	ruleType := os.Getenv("APICURIO_TEST_RULE_TYPE")
	if ruleType == "" {
		ruleType = "COMPATIBILITY"
	}

	resourceName := "data.apicurio_rule.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccApicurioDataSourceRuleArtifactMissingConfig(endpoint, groupID, artifactID, ruleType),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enabled", "false"),
					resource.TestCheckResourceAttr(resourceName, "scope", "artifact"),
					resource.TestCheckResourceAttr(resourceName, "group_id", groupID),
					resource.TestCheckResourceAttr(resourceName, "artifact_id", artifactID),
					resource.TestCheckResourceAttr(resourceName, "rule_type", ruleType),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
		},
	})
}

func TestAccApicurioDataSourceRule_global_exists(t *testing.T) {
	endpoint := os.Getenv("APICURIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("set APICURIO_ENDPOINT to run acceptance tests")
	}

	if os.Getenv("APICURIO_TEST_RULES") != "1" {
		t.Skip("set APICURIO_TEST_RULES=1 to run rule acceptance tests")
	}

	ruleType := os.Getenv("APICURIO_TEST_RULE_TYPE")
	if ruleType == "" {
		ruleType = "COMPATIBILITY"
	}
	ruleConfig := os.Getenv("APICURIO_TEST_GLOBAL_RULE_CONFIG1")
	if ruleConfig == "" {
		ruleConfig = "NONE"
	}

	resourceName := "data.apicurio_rule.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccApicurioDataSourceRuleGlobalConfig(endpoint, ruleType, ruleConfig),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "scope", "global"),
					resource.TestCheckResourceAttr(resourceName, "rule_type", ruleType),
					resource.TestCheckResourceAttr(resourceName, "enabled", "true"),
					resource.TestCheckResourceAttr(resourceName, "config", ruleConfig),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
		},
	})
}

func testAccApicurioDataSourceRuleArtifactConfig(endpoint, groupID, artifactID, content, ruleType, ruleConfig string) string {
	authHeader := os.Getenv("APICURIO_AUTH_HEADER")
	token := os.Getenv("APICURIO_TOKEN")
	oidcTokenURL := os.Getenv("APICURIO_OIDC_TOKEN_URL")
	oidcClientID := os.Getenv("APICURIO_OIDC_CLIENT_ID")
	oidcClientSecret := os.Getenv("APICURIO_OIDC_CLIENT_SECRET")
	apiVersion := os.Getenv("APICURIO_API_VERSION")

	authBlock := ""
	if authHeader != "" {
		authBlock = fmt.Sprintf("auth_header = %q", authHeader)
	} else if token != "" {
		authBlock = fmt.Sprintf("token = %q", token)
	} else if oidcTokenURL != "" && oidcClientID != "" && oidcClientSecret != "" {
		authBlock = fmt.Sprintf("oidc = { token_url = %q client_id = %q client_secret = %q }", oidcTokenURL, oidcClientID, oidcClientSecret)
	}

	apiVersionBlock := ""
	if apiVersion != "" {
		apiVersionBlock = fmt.Sprintf("api_version = %q", apiVersion)
	}

	return fmt.Sprintf(`
provider "apicurio" {
  endpoint = %q
  %s
		%s
}

resource "apicurio_artifact" "test" {
  group_id      = %q
  artifact_id   = %q
  artifact_type = "AVRO"
  version       = "v1"
  content       = %q
  hard_delete   = true
}

resource "apicurio_rule" "test" {
  scope       = "artifact"
  group_id    = apicurio_artifact.test.group_id
  artifact_id = apicurio_artifact.test.artifact_id
  rule_type   = %q
  config      = %q
}

data "apicurio_rule" "test" {
  scope       = "artifact"
  group_id    = apicurio_artifact.test.group_id
  artifact_id = apicurio_artifact.test.artifact_id
  rule_type   = %q

	depends_on = [apicurio_rule.test]
}
`, endpoint, authBlock, apiVersionBlock, groupID, artifactID, content, ruleType, ruleConfig, ruleType)
}

func testAccApicurioDataSourceRuleArtifactMissingConfig(endpoint, groupID, artifactID, ruleType string) string {
	authHeader := os.Getenv("APICURIO_AUTH_HEADER")
	token := os.Getenv("APICURIO_TOKEN")
	apiVersion := os.Getenv("APICURIO_API_VERSION")

	authBlock := ""
	if authHeader != "" {
		authBlock = fmt.Sprintf("auth_header = %q", authHeader)
	} else if token != "" {
		authBlock = fmt.Sprintf("token = %q", token)
	}

	apiVersionBlock := ""
	if apiVersion != "" {
		apiVersionBlock = fmt.Sprintf("api_version = %q", apiVersion)
	}

	return fmt.Sprintf(`
provider "apicurio" {
  endpoint = %q
  %s
		%s
}

data "apicurio_rule" "test" {
  scope       = "artifact"
  group_id    = %q
  artifact_id = %q
  rule_type   = %q
}
`, endpoint, authBlock, apiVersionBlock, groupID, artifactID, ruleType)
}

func testAccApicurioDataSourceRuleGlobalConfig(endpoint, ruleType, ruleConfig string) string {
	authHeader := os.Getenv("APICURIO_AUTH_HEADER")
	token := os.Getenv("APICURIO_TOKEN")
	apiVersion := os.Getenv("APICURIO_API_VERSION")

	authBlock := ""
	if authHeader != "" {
		authBlock = fmt.Sprintf("auth_header = %q", authHeader)
	} else if token != "" {
		authBlock = fmt.Sprintf("token = %q", token)
	}

	apiVersionBlock := ""
	if apiVersion != "" {
		apiVersionBlock = fmt.Sprintf("api_version = %q", apiVersion)
	}
	return fmt.Sprintf(`
provider "apicurio" {
  endpoint = %q
  %s
		%s
}

resource "apicurio_rule" "test" {
  scope     = "global"
  rule_type = %q
  config    = %q
}

data "apicurio_rule" "test" {
  scope     = "global"
  rule_type = %q

	depends_on = [apicurio_rule.test]
}
`, endpoint, authBlock, apiVersionBlock, ruleType, ruleConfig, ruleType)
}
