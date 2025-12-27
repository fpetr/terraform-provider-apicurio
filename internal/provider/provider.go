// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fpetr/terraform-provider-apicurio/internal/client"
)

var _ provider.Provider = &ApicurioProvider{}

type ApicurioProvider struct {
	version string
}

type ApicurioProviderModel struct {
	Endpoint           types.String    `tfsdk:"endpoint"`
	ApiVersion         types.String    `tfsdk:"api_version"`
	AuthHeader         types.String    `tfsdk:"auth_header"`
	Token              types.String    `tfsdk:"token"`
	BasicAuth          *basicAuthModel `tfsdk:"basic_auth"`
	OIDC               *oidcAuthModel  `tfsdk:"oidc"`
	InsecureSkipVerify types.Bool      `tfsdk:"insecure_skip_verify"`
	CABundlePath       types.String    `tfsdk:"ca_bundle_path"`
}

type basicAuthModel struct {
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

type oidcAuthModel struct {
	TokenURL     types.String `tfsdk:"token_url"`
	ClientID     types.String `tfsdk:"client_id"`
	ClientSecret types.String `tfsdk:"client_secret"`
	Scopes       types.List   `tfsdk:"scopes"`
	ExtraParams  types.Map    `tfsdk:"extra_params"`
	AuthStyle    types.String `tfsdk:"auth_style"`
}

func (p *ApicurioProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "apicurio"
	resp.Version = p.version
}

func (p *ApicurioProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Terraform provider for Apicurio Registry (v2/v3).",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Apicurio Registry base URL, e.g. `http://localhost:3080`.",
				Required:            true,
			},
			"api_version": schema.StringAttribute{
				MarkdownDescription: "Apicurio API flavor to use. One of: `v2`, `v3`. If unset, the provider probes for v3 and falls back to v2 (best-effort).",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("v2", "v3"),
				},
			},
			"auth_header": schema.StringAttribute{
				MarkdownDescription: "Optional literal header in the form `Header-Name: value`, e.g. `Authorization: Bearer <token>`. Overrides `token` if set.",
				Optional:            true,
				Sensitive:           true,
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "Optional bearer token. If set, provider sends `Authorization: Bearer <token>`.",
				Optional:            true,
				Sensitive:           true,
			},
			"basic_auth": schema.SingleNestedAttribute{
				MarkdownDescription: "Optional HTTP Basic authentication.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"username": schema.StringAttribute{
						MarkdownDescription: "Basic auth username.",
						Required:            true,
					},
					"password": schema.StringAttribute{
						MarkdownDescription: "Basic auth password.",
						Required:            true,
						Sensitive:           true,
					},
				},
			},
			"oidc": schema.SingleNestedAttribute{
				MarkdownDescription: "Optional OpenID Connect (Keycloak) client credentials flow.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"token_url": schema.StringAttribute{
						MarkdownDescription: "Token endpoint URL (for Keycloak: `<issuer>/protocol/openid-connect/token`).",
						Required:            true,
					},
					"client_id": schema.StringAttribute{
						MarkdownDescription: "OIDC client id.",
						Required:            true,
					},
					"client_secret": schema.StringAttribute{
						MarkdownDescription: "OIDC client secret.",
						Required:            true,
						Sensitive:           true,
					},
					"scopes": schema.ListAttribute{
						MarkdownDescription: "Optional OAuth2 scopes.",
						Optional:            true,
						ElementType:         types.StringType,
					},
					"extra_params": schema.MapAttribute{
						MarkdownDescription: "Optional extra token endpoint parameters (e.g. audience).",
						Optional:            true,
						ElementType:         types.StringType,
					},
					"auth_style": schema.StringAttribute{
						MarkdownDescription: "Optional client authentication style: `auto`, `in_header`, or `in_params`. Defaults to `auto`.",
						Optional:            true,
						Validators: []validator.String{
							stringvalidator.OneOf("auto", "in_header", "in_params"),
						},
					},
				},
			},
			"insecure_skip_verify": schema.BoolAttribute{
				MarkdownDescription: "If true, TLS certificates are not verified (use only for dev/test).",
				Optional:            true,
			},
			"ca_bundle_path": schema.StringAttribute{
				MarkdownDescription: "Optional path to a PEM CA bundle to trust when connecting to HTTPS endpoints.",
				Optional:            true,
			},
		},
	}
}

func (p *ApicurioProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data ApicurioProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiVersion := "v3"
	if !data.ApiVersion.IsNull() && !data.ApiVersion.IsUnknown() {
		apiVersion = data.ApiVersion.ValueString()
	}

	cfg := client.ClientConfig{
		Endpoint:           data.Endpoint.ValueString(),
		APIVersion:         client.ServerFlavor(apiVersion),
		AuthHeader:         data.AuthHeader.ValueString(),
		Token:              data.Token.ValueString(),
		InsecureSkipVerify: !data.InsecureSkipVerify.IsNull() && data.InsecureSkipVerify.ValueBool(),
		CABundlePath:       data.CABundlePath.ValueString(),
		Timeout:            30 * time.Second,
	}

	if data.BasicAuth != nil {
		cfg.BasicAuth = &client.BasicAuthConfig{
			Username: data.BasicAuth.Username.ValueString(),
			Password: data.BasicAuth.Password.ValueString(),
		}
	}

	if data.OIDC != nil {
		var scopes []string
		if !data.OIDC.Scopes.IsNull() && !data.OIDC.Scopes.IsUnknown() {
			_ = data.OIDC.Scopes.ElementsAs(ctx, &scopes, false)
		}
		extra := map[string]string{}
		if !data.OIDC.ExtraParams.IsNull() && !data.OIDC.ExtraParams.IsUnknown() {
			_ = data.OIDC.ExtraParams.ElementsAs(ctx, &extra, false)
		}
		authStyle := strings.TrimSpace(data.OIDC.AuthStyle.ValueString())
		if authStyle == "" {
			authStyle = "auto"
		}

		cfg.OIDC = &client.OIDCConfig{
			TokenURL:     data.OIDC.TokenURL.ValueString(),
			ClientID:     data.OIDC.ClientID.ValueString(),
			ClientSecret: data.OIDC.ClientSecret.ValueString(),
			Scopes:       scopes,
			ExtraParams:  extra,
			AuthStyle:    authStyle,
		}
	}

	resp.Diagnostics.Append(validateProviderConfig(cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	regClient, diags := client.New(ctx, cfg)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.ResourceData = regClient
	resp.DataSourceData = regClient
}

func validateProviderConfig(cfg client.ClientConfig) diag.Diagnostics {
	var diags diag.Diagnostics
	if err := client.ValidateAuthHeader(cfg.AuthHeader); err != nil {
		diags.AddError("Invalid auth_header", err.Error())
	}

	authMethods := 0
	if strings.TrimSpace(cfg.AuthHeader) != "" {
		authMethods++
	}
	if strings.TrimSpace(cfg.Token) != "" {
		authMethods++
	}
	if cfg.BasicAuth != nil {
		authMethods++
		if strings.TrimSpace(cfg.BasicAuth.Username) == "" || strings.TrimSpace(cfg.BasicAuth.Password) == "" {
			diags.AddError("Invalid basic_auth", "basic_auth.username and basic_auth.password must be set")
		}
	}
	if cfg.OIDC != nil {
		authMethods++
		if strings.TrimSpace(cfg.OIDC.TokenURL) == "" || strings.TrimSpace(cfg.OIDC.ClientID) == "" || strings.TrimSpace(cfg.OIDC.ClientSecret) == "" {
			diags.AddError("Invalid oidc", "oidc.token_url, oidc.client_id, and oidc.client_secret must be set")
		}
	}
	if authMethods > 1 {
		diags.AddError(
			"Invalid authentication configuration",
			fmt.Sprintf("configure only one of auth_header, token, basic_auth, or oidc (got %d)", authMethods),
		)
	}
	return diags
}

func (p *ApicurioProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewArtifactResource,
		NewRuleResource,
	}
}

func (p *ApicurioProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewArtifactDataSource,
		NewRuleDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &ApicurioProvider{version: version}
	}
}
