// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccApicurioArtifact_basic(t *testing.T) {
	endpoint := os.Getenv("APICURIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("set APICURIO_ENDPOINT to run acceptance tests")
	}

	groupID := os.Getenv("APICURIO_TEST_GROUP")
	if groupID == "" {
		groupID = "com.example.common.v1"
	}

	artifactID := fmt.Sprintf("tf-acc-%d", time.Now().UnixNano())
	content1 := `{"type":"record","name":"ErrorCommonMessage","namespace":"com.example.common.v1","fields":[{"name":"message","type":"string"}]}`
	content2 := `{"type":"record","name":"ErrorCommonMessage","namespace":"com.example.common.v1","fields":[{"name":"message","type":"string"},{"name":"code","type":"string","default":""}]}`
	content1Hash := sha256hexCanonicalString(t, content1)
	content2Hash := sha256hexCanonicalString(t, content2)

	resourceName := "apicurio_artifact.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccApicurioArtifactConfig(endpoint, groupID, artifactID, content1, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "group_id", groupID),
					resource.TestCheckResourceAttr(resourceName, "artifact_id", artifactID),
					resource.TestCheckResourceAttr(resourceName, "artifact_type", "AVRO"),
					resource.TestCheckResourceAttr(resourceName, "version", "v1"),
					resource.TestCheckResourceAttr(resourceName, "allow_overwrite_version", "false"),
					resource.TestCheckResourceAttr(resourceName, "hard_delete", "true"),
					resource.TestCheckResourceAttrSet(resourceName, "name"),
					resource.TestCheckResourceAttr(resourceName, "content_canonical_sha256", content1Hash),
					resource.TestCheckTypeSetElemAttr(resourceName, "labels.*", "com.example.control.pravidla.otk.v1.public.error"),
					resource.TestCheckTypeSetElemAttr(resourceName, "labels.*", "com.example.control.pravidla.ai.v1.public.error"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateId:     fmt.Sprintf("%s/%s", groupID, artifactID),
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					// Config-only inputs can't be inferred during import.
					"allow_overwrite_version",
					"hard_delete",
					"artifact_type",
					"version",
					"content",
				},
			},
			{
				Config: testAccApicurioArtifactConfig(endpoint, groupID, artifactID, content2, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "allow_overwrite_version", "true"),
					resource.TestCheckResourceAttr(resourceName, "content_canonical_sha256", content2Hash),
				),
			},
		},
	})
}

func TestAccApicurioArtifact_contentFileAndMetadataOnlyUpdate(t *testing.T) {
	endpoint := os.Getenv("APICURIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("set APICURIO_ENDPOINT to run acceptance tests")
	}

	groupID := os.Getenv("APICURIO_TEST_GROUP")
	if groupID == "" {
		groupID = "com.example.common.v1"
	}

	artifactID := fmt.Sprintf("tf-acc-file-%d", time.Now().UnixNano())
	content := `{"type":"record","name":"AccContentFile","namespace":"com.example.common.v1","fields":[{"name":"message","type":"string"}]}`
	contentHash := sha256hexCanonicalString(t, content)

	tmpDir := t.TempDir()
	contentPath := filepath.Join(tmpDir, "schema.avsc")
	if err := os.WriteFile(contentPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write temp content file: %v", err)
	}

	resourceName := "apicurio_artifact.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccApicurioArtifactContentFileConfig(endpoint, groupID, artifactID, contentPath, "", "", []string{" label.one ", "label.two", "label.two"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "name"),
					resource.TestCheckResourceAttr(resourceName, "content_canonical_sha256", contentHash),
					resource.TestCheckTypeSetElemAttr(resourceName, "labels.*", "label.one"),
					resource.TestCheckTypeSetElemAttr(resourceName, "labels.*", "label.two"),
				),
			},
			{
				// Metadata-only update: keep content_file identical.
				Config: testAccApicurioArtifactContentFileConfig(endpoint, groupID, artifactID, contentPath, "Custom Name", "Custom Description", []string{"label.two"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "Custom Name"),
					resource.TestCheckResourceAttr(resourceName, "description", "Custom Description"),
					resource.TestCheckResourceAttr(resourceName, "content_canonical_sha256", contentHash),
					resource.TestCheckTypeSetElemAttr(resourceName, "labels.*", "label.two"),
				),
			},
		},
	})
}

func sha256hexCanonicalString(t *testing.T, s string) string {
	t.Helper()
	canon, err := canonicalizeJSON([]byte(s))
	if err != nil {
		t.Fatalf("expected valid JSON content, got error: %v", err)
	}
	return sha256hex(canon)
}

func TestAccApicurioArtifact_overwriteVersionGuard(t *testing.T) {
	endpoint := os.Getenv("APICURIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("set APICURIO_ENDPOINT to run acceptance tests")
	}

	groupID := os.Getenv("APICURIO_TEST_GROUP")
	if groupID == "" {
		groupID = "com.example.common.v1"
	}

	artifactID := fmt.Sprintf("tf-acc-ovw-%d", time.Now().UnixNano())
	content1 := `{"type":"record","name":"OverwriteAcc","namespace":"com.example.common.v1","fields":[{"name":"message","type":"string"}]}`
	content2 := `{"type":"record","name":"OverwriteAcc","namespace":"com.example.common.v1","fields":[{"name":"message","type":"string"},{"name":"extra","type":"string","default":""}]}`

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccApicurioArtifactConfig(endpoint, groupID, artifactID, content1, false),
			},
			{
				Config:      testAccApicurioArtifactConfig(endpoint, groupID, artifactID, content2, false),
				ExpectError: regexp.MustCompile("Version already exists"),
			},
			{
				Config: testAccApicurioArtifactConfig(endpoint, groupID, artifactID, content2, true),
			},
		},
	})
}
func testAccApicurioArtifactConfig(endpoint, groupID, artifactID, content string, allowOverwrite bool) string {
	// Auth is optional; set APICURIO_AUTH_HEADER, APICURIO_TOKEN, or APICURIO_OIDC_* as needed.
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

  labels = [
		"com.example.control.pravidla.otk.v1.public.error",
		"com.example.control.pravidla.ai.v1.public.error",
  ]

  allow_overwrite_version = %t
  hard_delete             = true
}
`, endpoint, authBlock, apiVersionBlock, groupID, artifactID, content, allowOverwrite)
}

func testAccApicurioArtifactContentFileConfig(endpoint, groupID, artifactID, contentFilePath, name, description string, labels []string) string {
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

	labelsHCL := "[]"
	if len(labels) > 0 {
		labelsHCL = "[\n"
		for _, l := range labels {
			l = strings.TrimSpace(l)
			labelsHCL += fmt.Sprintf("    %q,\n", l)
		}
		labelsHCL += "  ]"
	}

	nameLine := ""
	if name != "" {
		nameLine = fmt.Sprintf("name = %q", name)
	}
	descLine := ""
	if description != "" {
		descLine = fmt.Sprintf("description = %q", description)
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

  content_file = %q

  %s
  %s
  labels = %s

  hard_delete = true
}
`, endpoint, authBlock, apiVersionBlock, groupID, artifactID, contentFilePath, nameLine, descLine, labelsHCL)
}
