package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccApicurioArtifact_groupCleanup_hardDelete(t *testing.T) {
	endpoint := os.Getenv("APICURIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("set APICURIO_ENDPOINT to run acceptance tests")
	}

	groupID := fmt.Sprintf("tf-acc-gc-%d", time.Now().UnixNano())
	artifactA := fmt.Sprintf("tf-acc-a-%d", time.Now().UnixNano())
	artifactB := fmt.Sprintf("tf-acc-b-%d", time.Now().UnixNano())

	content := `{"type":"record","name":"AccCleanup","namespace":"com.example","fields":[{"name":"message","type":"string"}]}`

	resourceA := "apicurio_artifact.a"
	resourceB := "apicurio_artifact.b"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccApicurioArtifactTwoInGroupConfig(endpoint, groupID, artifactA, artifactB, content),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceA, "group_id", groupID),
					resource.TestCheckResourceAttr(resourceB, "group_id", groupID),
					checkAccGroupExists(endpoint, groupID, true),
				),
			},
			{
				// Remove artifact A; group must remain since artifact B still exists.
				Config: testAccApicurioArtifactOneInGroupConfig(endpoint, groupID, artifactB, content),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceB, "group_id", groupID),
					checkAccGroupHasAnyArtifacts(endpoint, groupID, true),
					checkAccGroupExists(endpoint, groupID, true),
				),
			},
			{
				// Remove the last artifact; best-effort cleanup should delete the group.
				Config: testAccApicurioProviderOnlyConfig(endpoint),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAccGroupHasAnyArtifacts(endpoint, groupID, false),
					checkAccGroupExists(endpoint, groupID, false),
				),
			},
		},
	})
}

func testAccApicurioProviderOnlyConfig(endpoint string) string {
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
`, endpoint, authBlock, apiVersionBlock)
}

func testAccApicurioArtifactTwoInGroupConfig(endpoint, groupID, artifactA, artifactB, content string) string {
	return testAccApicurioProviderOnlyConfig(endpoint) + fmt.Sprintf(`
resource "apicurio_artifact" "a" {
  group_id      = %q
  artifact_id   = %q
  artifact_type = "AVRO"
  version       = "v1"
  content       = %q
  hard_delete   = true
}

resource "apicurio_artifact" "b" {
  group_id      = %q
  artifact_id   = %q
  artifact_type = "AVRO"
  version       = "v1"
  content       = %q
  hard_delete   = true
}
`, groupID, artifactA, content, groupID, artifactB, content)
}

func testAccApicurioArtifactOneInGroupConfig(endpoint, groupID, artifactB, content string) string {
	return testAccApicurioProviderOnlyConfig(endpoint) + fmt.Sprintf(`
resource "apicurio_artifact" "b" {
  group_id      = %q
  artifact_id   = %q
  artifact_type = "AVRO"
  version       = "v1"
  content       = %q
  hard_delete   = true
}
`, groupID, artifactB, content)
}

type accHTTP struct {
	endpoint   string
	authHeader string
	token      string
	client     *http.Client
}

func newAccHTTP(endpoint string) *accHTTP {
	return &accHTTP{
		endpoint:   strings.TrimRight(strings.TrimSpace(endpoint), "/"),
		authHeader: strings.TrimSpace(os.Getenv("APICURIO_AUTH_HEADER")),
		token:      strings.TrimSpace(os.Getenv("APICURIO_TOKEN")),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (a *accHTTP) do(ctx context.Context, method, path string, query url.Values) (int, string, error) {
	u, err := url.Parse(a.endpoint + path)
	if err != nil {
		return 0, "", err
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return 0, "", err
	}

	if a.authHeader != "" {
		parts := strings.SplitN(a.authHeader, ":", 2)
		if len(parts) == 2 {
			req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	} else if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), nil
}

func checkAccGroupHasAnyArtifacts(endpoint, groupID string, expected bool) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		a := newAccHTTP(endpoint)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		q := url.Values{}
		q.Set("limit", "1")
		status, body, err := a.do(ctx, http.MethodGet, "/apis/registry/v2/groups/"+url.PathEscape(groupID)+"/artifacts", q)
		if err != nil {
			return err
		}
		if status == http.StatusNotFound {
			if expected {
				return fmt.Errorf("expected group to have artifacts, got 404")
			}
			return nil
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("unexpected status %d: %s", status, body)
		}

		hasAny, err := accParseHasAnyArtifacts(body)
		if err != nil {
			return err
		}
		if hasAny != expected {
			return fmt.Errorf("expected hasAny=%v, got %v (body=%s)", expected, hasAny, body)
		}
		return nil
	}
}

func checkAccGroupExists(endpoint, groupID string, expected bool) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		a := newAccHTTP(endpoint)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		q := url.Values{}
		q.Set("limit", "1000")
		status, body, err := a.do(ctx, http.MethodGet, "/apis/registry/v2/groups", q)
		if err != nil {
			return err
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("unexpected status %d: %s", status, body)
		}

		ids, err := accParseGroupIDs(body)
		if err != nil {
			return err
		}
		found := false
		for _, id := range ids {
			if id == groupID {
				found = true
				break
			}
		}
		if found != expected {
			return fmt.Errorf("expected groupExists=%v, got %v (knownGroups=%d)", expected, found, len(ids))
		}
		return nil
	}
}

func accParseHasAnyArtifacts(body string) (bool, error) {
	var decoded any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		return false, fmt.Errorf("unable to parse artifacts response: %w", err)
	}

	switch v := decoded.(type) {
	case []any:
		return len(v) > 0, nil
	case map[string]any:
		if artifacts, ok := v["artifacts"].([]any); ok {
			return len(artifacts) > 0, nil
		}
		if count, ok := v["count"].(float64); ok {
			return count > 0, nil
		}
		return false, nil
	default:
		return false, nil
	}
}

func accParseGroupIDs(body string) ([]string, error) {
	var decoded any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		return nil, fmt.Errorf("unable to parse groups response: %w", err)
	}

	collect := func(items []any) []string {
		out := make([]string, 0, len(items))
		for _, it := range items {
			switch x := it.(type) {
			case string:
				if strings.TrimSpace(x) != "" {
					out = append(out, x)
				}
			case map[string]any:
				// common patterns: {"groupId":"..."} or {"id":"..."}
				if s, ok := x["groupId"].(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, s)
					continue
				}
				if s, ok := x["id"].(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, s)
					continue
				}
			}
		}
		return out
	}

	switch v := decoded.(type) {
	case []any:
		return collect(v), nil
	case map[string]any:
		// common patterns: {"groups": [...], "count": N}
		if groups, ok := v["groups"].([]any); ok {
			return collect(groups), nil
		}
		if items, ok := v["items"].([]any); ok {
			return collect(items), nil
		}
		return []string{}, nil
	default:
		return []string{}, nil
	}
}
