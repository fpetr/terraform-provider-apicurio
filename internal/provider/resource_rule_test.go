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

func TestAccApicurioRule_artifact(t *testing.T) {
	endpoint := os.Getenv("APICURIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("set APICURIO_ENDPOINT to run acceptance tests")
	}

	// Rules tests are opt-in since they mutate registry behavior.
	if os.Getenv("APICURIO_TEST_RULES") != "1" {
		t.Skip("set APICURIO_TEST_RULES=1 to run rule acceptance tests")
	}

	groupID := os.Getenv("APICURIO_TEST_GROUP")
	if groupID == "" {
		groupID = "tf.acc.rules"
	}

	artifactID := fmt.Sprintf("tf-acc-rule-%d", time.Now().UnixNano())
	content := `{"type":"record","name":"RuleAccTest","namespace":"tf.acc.rules","fields":[{"name":"message","type":"string"}]}`

	ruleType := os.Getenv("APICURIO_TEST_RULE_TYPE")
	if ruleType == "" {
		ruleType = "COMPATIBILITY"
	}
	config1 := os.Getenv("APICURIO_TEST_RULE_CONFIG1")
	if config1 == "" {
		config1 = "BACKWARD"
	}
	config2 := os.Getenv("APICURIO_TEST_RULE_CONFIG2")
	if config2 == "" {
		config2 = "FULL"
	}

	resourceName := "apicurio_rule.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccApicurioRuleArtifactConfig(endpoint, groupID, artifactID, content, ruleType, config1),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateId:     fmt.Sprintf("%s/%s/%s", groupID, artifactID, ruleType),
				ImportStateVerify: true,
			},
			{
				Config: testAccApicurioRuleArtifactConfig(endpoint, groupID, artifactID, content, ruleType, config2),
			},
			{
				Config: testAccApicurioRuleArtifactDisabledConfig(endpoint, groupID, artifactID, content, ruleType),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enabled", "false"),
				),
			},
			{
				Config: testAccApicurioRuleArtifactConfig(endpoint, groupID, artifactID, content, ruleType, config1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enabled", "true"),
					resource.TestCheckResourceAttr(resourceName, "config", config1),
				),
			},
		},
	})
}

func TestAccApicurioRule_global(t *testing.T) {
	endpoint := os.Getenv("APICURIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("set APICURIO_ENDPOINT to run acceptance tests")
	}

	// Rules tests are opt-in since they mutate registry behavior.
	if os.Getenv("APICURIO_TEST_RULES") != "1" {
		t.Skip("set APICURIO_TEST_RULES=1 to run rule acceptance tests")
	}

	ruleType := os.Getenv("APICURIO_TEST_RULE_TYPE")
	if ruleType == "" {
		ruleType = "COMPATIBILITY"
	}
	config1 := os.Getenv("APICURIO_TEST_GLOBAL_RULE_CONFIG1")
	if config1 == "" {
		config1 = "NONE"
	}
	config2 := os.Getenv("APICURIO_TEST_GLOBAL_RULE_CONFIG2")
	if config2 == "" {
		config2 = "BACKWARD"
	}

	resourceName := "apicurio_rule.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccApicurioRuleGlobalConfig(endpoint, ruleType, config1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "scope", "global"),
					resource.TestCheckResourceAttr(resourceName, "rule_type", ruleType),
					resource.TestCheckResourceAttr(resourceName, "config", config1),
					resource.TestCheckResourceAttr(resourceName, "enabled", "true"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateId:     fmt.Sprintf("global/%s", ruleType),
				ImportStateVerify: true,
			},
			{
				Config: testAccApicurioRuleGlobalConfig(endpoint, ruleType, config2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "config", config2),
				),
			},
			{
				Config: testAccApicurioRuleGlobalDisabledConfig(endpoint, ruleType),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enabled", "false"),
				),
			},
			{
				Config: testAccApicurioRuleGlobalConfig(endpoint, ruleType, config1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enabled", "true"),
					resource.TestCheckResourceAttr(resourceName, "config", config1),
				),
			},
		},
	})
}

func testAccApicurioRuleArtifactConfig(endpoint, groupID, artifactID, content, ruleType, ruleConfig string) string {
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

	content = %q

	hard_delete = true
}

resource "apicurio_rule" "test" {
	scope       = "artifact"
	group_id    = apicurio_artifact.test.group_id
	artifact_id = apicurio_artifact.test.artifact_id

	rule_type = %q
	config    = %q
}
`, endpoint, authBlock, apiVersionBlock, groupID, artifactID, content, ruleType, ruleConfig)
}

func testAccApicurioRuleArtifactDisabledConfig(endpoint, groupID, artifactID, content, ruleType string) string {
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

	content = %q

	hard_delete = true
}

resource "apicurio_rule" "test" {
	scope       = "artifact"
	group_id    = apicurio_artifact.test.group_id
	artifact_id = apicurio_artifact.test.artifact_id

	rule_type = %q
	config    = "IGNORED"
	enabled   = false
}
`, endpoint, authBlock, apiVersionBlock, groupID, artifactID, content, ruleType)
}

func testAccApicurioRuleGlobalConfig(endpoint, ruleType, ruleConfig string) string {
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
`, endpoint, authBlock, apiVersionBlock, ruleType, ruleConfig)
}

func testAccApicurioRuleGlobalDisabledConfig(endpoint, ruleType string) string {
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
	config    = "IGNORED"
	enabled   = false
}
`, endpoint, authBlock, apiVersionBlock, ruleType)
}
