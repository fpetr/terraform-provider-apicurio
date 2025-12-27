package provider

import (
	"testing"

	"github.com/fpetr/terraform-provider-apicurio/internal/client"
)

func TestValidateProviderConfig_AllowsNoAuth(t *testing.T) {
	cfg := client.ClientConfig{Endpoint: "http://example"}
	d := validateProviderConfig(cfg)
	if d.HasError() {
		t.Fatalf("expected no errors, got: %v", d)
	}
}

func TestValidateProviderConfig_InvalidAuthHeader(t *testing.T) {
	cfg := client.ClientConfig{Endpoint: "http://example", AuthHeader: "not-a-header"}
	d := validateProviderConfig(cfg)
	if !d.HasError() {
		t.Fatalf("expected errors")
	}
}

func TestValidateProviderConfig_BasicAuthRequiresUserPass(t *testing.T) {
	cfg := client.ClientConfig{Endpoint: "http://example", BasicAuth: &client.BasicAuthConfig{Username: "", Password: "p"}}
	d := validateProviderConfig(cfg)
	if !d.HasError() {
		t.Fatalf("expected errors")
	}
}

func TestValidateProviderConfig_OIDCRequiresFields(t *testing.T) {
	cfg := client.ClientConfig{Endpoint: "http://example", OIDC: &client.OIDCConfig{TokenURL: "", ClientID: "", ClientSecret: ""}}
	d := validateProviderConfig(cfg)
	if !d.HasError() {
		t.Fatalf("expected errors")
	}
}

func TestValidateProviderConfig_RejectsMultipleAuthMethods(t *testing.T) {
	cfg := client.ClientConfig{
		Endpoint:   "http://example",
		AuthHeader: "Authorization: Bearer x",
		Token:      "x",
	}
	d := validateProviderConfig(cfg)
	if !d.HasError() {
		t.Fatalf("expected errors")
	}
}

func TestValidateProviderConfig_RejectsTokenAndBasicAuth(t *testing.T) {
	cfg := client.ClientConfig{
		Endpoint: "http://example",
		Token:    "x",
		BasicAuth: &client.BasicAuthConfig{
			Username: "u",
			Password: "p",
		},
	}
	d := validateProviderConfig(cfg)
	if !d.HasError() {
		t.Fatalf("expected errors")
	}
}
