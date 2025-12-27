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

func TestAccApicurioDataSourceArtifact_basic(t *testing.T) {
	endpoint := os.Getenv("APICURIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("set APICURIO_ENDPOINT to run acceptance tests")
	}

	groupID := os.Getenv("APICURIO_TEST_GROUP")
	if groupID == "" {
		groupID = "com.example.common.v1"
	}

	artifactID := fmt.Sprintf("tf-ds-acc-%d", time.Now().UnixNano())
	content := `{"type":"record","name":"DsAccTest","namespace":"com.example.common.v1","fields":[{"name":"message","type":"string"}]}`

	resourceName := "data.apicurio_artifact.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccApicurioDataSourceArtifactConfig(endpoint, groupID, artifactID, content),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "group_id", groupID),
					resource.TestCheckResourceAttr(resourceName, "artifact_id", artifactID),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "content_sha256"),
				),
			},
		},
	})
}

func testAccApicurioDataSourceArtifactConfig(endpoint, groupID, artifactID, content string) string {
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

data "apicurio_artifact" "test" {
  group_id    = apicurio_artifact.test.group_id
  artifact_id = apicurio_artifact.test.artifact_id
}
`, endpoint, authBlock, apiVersionBlock, groupID, artifactID, content)
}
