package provider

import (
	"context"
	"os"
	"strconv"
	"time"

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
	BaseURL        types.String `tfsdk:"base_url"`
	Username       types.String `tfsdk:"username"`
	Password       types.String `tfsdk:"password"`
	MaxRetries     types.Int64  `tfsdk:"max_retries"`
	BaseBackoffMS  types.Int64  `tfsdk:"base_backoff_ms"`
	MaxBackoffMS   types.Int64  `tfsdk:"max_backoff_ms"`
	RequestTimeout types.Int64  `tfsdk:"request_timeout_ms"`
}

type providerConfig struct {
	BaseURL        string
	Username       string
	Password       string
	MaxRetries     int
	BaseBackoff    time.Duration
	MaxBackoff     time.Duration
	RequestTimeout time.Duration
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
		"base_url":           schema.StringAttribute{Optional: true, Description: "Openprovider REST API base URL. Env: OPENPROVIDER_MAIN_API_URL"},
		"username":           schema.StringAttribute{Optional: true, Description: "Openprovider username. Env: OPENPROVIDER_MAIN_USERNAME"},
		"password":           schema.StringAttribute{Optional: true, Sensitive: true, Description: "Openprovider password. Env: OPENPROVIDER_MAIN_PASSWORD"},
		"max_retries":        schema.Int64Attribute{Optional: true, Description: "Retry count for 429/5xx/network errors. Default: 6"},
		"base_backoff_ms":    schema.Int64Attribute{Optional: true, Description: "Base backoff in milliseconds. Default: 1500"},
		"max_backoff_ms":     schema.Int64Attribute{Optional: true, Description: "Max backoff in milliseconds. Default: 20000"},
		"request_timeout_ms": schema.Int64Attribute{Optional: true, Description: "HTTP request timeout in milliseconds. Default: 45000"},
	}}
}

func (p *openproviderProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg openproviderProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.BaseURL.IsUnknown() || cfg.Username.IsUnknown() || cfg.Password.IsUnknown() || cfg.MaxRetries.IsUnknown() || cfg.BaseBackoffMS.IsUnknown() || cfg.MaxBackoffMS.IsUnknown() || cfg.RequestTimeout.IsUnknown() {
		resp.Diagnostics.AddError("Unknown config", "Provider config has unknown values")
		return
	}

	baseURL := envOr("OPENPROVIDER_MAIN_API_URL", "https://api.openprovider.eu/v1beta")
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

	maxRetries := envInt("OPENPROVIDER_MAX_RETRIES", 6)
	if !cfg.MaxRetries.IsNull() {
		maxRetries = int(cfg.MaxRetries.ValueInt64())
	}
	baseBackoffMS := envInt("OPENPROVIDER_BASE_BACKOFF_MS", 1500)
	if !cfg.BaseBackoffMS.IsNull() {
		baseBackoffMS = int(cfg.BaseBackoffMS.ValueInt64())
	}
	maxBackoffMS := envInt("OPENPROVIDER_MAX_BACKOFF_MS", 20000)
	if !cfg.MaxBackoffMS.IsNull() {
		maxBackoffMS = int(cfg.MaxBackoffMS.ValueInt64())
	}
	timeoutMS := envInt("OPENPROVIDER_REQUEST_TIMEOUT_MS", 45000)
	if !cfg.RequestTimeout.IsNull() {
		timeoutMS = int(cfg.RequestTimeout.ValueInt64())
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

	pc := &providerConfig{
		BaseURL:        baseURL,
		Username:       username,
		Password:       password,
		MaxRetries:     maxRetries,
		BaseBackoff:    time.Duration(baseBackoffMS) * time.Millisecond,
		MaxBackoff:     time.Duration(maxBackoffMS) * time.Millisecond,
		RequestTimeout: time.Duration(timeoutMS) * time.Millisecond,
	}
	resp.ResourceData = pc
}

func (p *openproviderProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{NewDomainNameserversResource}
}

func (p *openproviderProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
