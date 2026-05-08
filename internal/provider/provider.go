package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &openproviderProvider{}

type openproviderProvider struct{ version string }

type openproviderProviderModel struct {
	BaseURL  types.String `tfsdk:"base_url"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

type providerConfig struct {
	BaseURL  string
	Username string
	Password string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &openproviderProvider{version: version} }
}

func (p *openproviderProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "openprovider"
	resp.Version = p.version
}

func (p *openproviderProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{Attributes: map[string]schema.Attribute{
		"base_url": schema.StringAttribute{Optional: true, Description: "Openprovider REST API base URL. Env: OPENPROVIDER_MAIN_API_URL"},
		"username": schema.StringAttribute{Optional: true, Description: "Openprovider username. Env: OPENPROVIDER_MAIN_USERNAME"},
		"password": schema.StringAttribute{Optional: true, Sensitive: true, Description: "Openprovider password. Env: OPENPROVIDER_MAIN_PASSWORD"},
	}}
}

func (p *openproviderProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg openproviderProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.BaseURL.IsUnknown() || cfg.Username.IsUnknown() || cfg.Password.IsUnknown() {
		resp.Diagnostics.AddError("Unknown config", "Provider config has unknown values")
		return
	}

	baseURL := os.Getenv("OPENPROVIDER_MAIN_API_URL")
	if !cfg.BaseURL.IsNull() {
		baseURL = cfg.BaseURL.ValueString()
	}
	username := os.Getenv("OPENPROVIDER_MAIN_USERNAME")
	if !cfg.Username.IsNull() {
		username = cfg.Username.ValueString()
	}
	password := os.Getenv("OPENPROVIDER_MAIN_PASSWORD")
	if !cfg.Password.IsNull() {
		password = cfg.Password.ValueString()
	}

	if baseURL == "" {
		baseURL = "https://api.openprovider.eu/v1beta"
	}
	if username == "" {
		resp.Diagnostics.AddAttributeError(path.Root("username"), "Missing username", "Set username or OPENPROVIDER_MAIN_USERNAME")
	}
	if password == "" {
		resp.Diagnostics.AddAttributeError(path.Root("password"), "Missing password", "Set password or OPENPROVIDER_MAIN_PASSWORD")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	pc := &providerConfig{BaseURL: baseURL, Username: username, Password: password}
	resp.ResourceData = pc
}

func (p *openproviderProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{NewDomainNameserversResource}
}

func (p *openproviderProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}
