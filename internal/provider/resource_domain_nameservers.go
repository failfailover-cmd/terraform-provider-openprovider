package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &domainNameserversResource{}
var _ resource.ResourceWithImportState = &domainNameserversResource{}

type domainNameserversResource struct{ cfg *providerConfig }

type domainNameserversModel struct {
	ID          types.String `tfsdk:"id"`
	Domain      types.String `tfsdk:"domain"`
	Nameservers types.List   `tfsdk:"nameservers"`
}

func NewDomainNameserversResource() resource.Resource { return &domainNameserversResource{} }

func (r *domainNameserversResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain_nameservers"
}

func (r *domainNameserversResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Attributes: map[string]schema.Attribute{
		"id":          schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"domain":      schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"nameservers": schema.ListAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.List{listplanmodifier.RequiresReplace()}},
	}}
}

func (r *domainNameserversResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	cfg, ok := req.ProviderData.(*providerConfig)
	if !ok {
		resp.Diagnostics.AddError("Unexpected config type", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	r.cfg = cfg
}

func (r *domainNameserversResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan domainNameserversModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.applyNS(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Openprovider API error", err.Error())
		return
	}
	plan.ID = plan.Domain
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *domainNameserversResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var st domainNameserversModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// API read for exact NS may vary; keep last-known state to avoid destructive drift.
	resp.Diagnostics.Append(resp.State.Set(ctx, &st)...)
}

func (r *domainNameserversResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan domainNameserversModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.applyNS(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Openprovider API error", err.Error())
		return
	}
	plan.ID = plan.Domain
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *domainNameserversResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// No safe rollback NS on delete.
}

func (r *domainNameserversResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("domain"), req, resp)
}

func (r *domainNameserversResource) applyNS(ctx context.Context, plan domainNameserversModel) error {
	token, err := r.login(ctx)
	if err != nil {
		return err
	}
	var ns []string
	if diags := plan.Nameservers.ElementsAs(ctx, &ns, false); diags.HasError() {
		return fmt.Errorf("invalid nameservers list")
	}
	parts := strings.SplitN(plan.Domain.ValueString(), ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid domain: %s", plan.Domain.ValueString())
	}
	type nsObj struct {
		Name string `json:"name"`
	}
	body := map[string]any{
		"name_servers": []nsObj{{Name: ns[0]}},
	}
	list := make([]nsObj, 0, len(ns))
	for _, n := range ns {
		list = append(list, nsObj{Name: n})
	}
	body["name_servers"] = list
	b, _ := json.Marshal(body)
	url := strings.TrimRight(r.cfg.BaseURL, "/") + "/domains/" + plan.Domain.ValueString()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	cli := &http.Client{Timeout: 40 * time.Second}
	res, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("PUT /domains failed: status=%d body=%s", res.StatusCode, string(raw))
	}
	return nil
}

func (r *domainNameserversResource) login(ctx context.Context) (string, error) {
	payload := map[string]string{"username": r.cfg.Username, "password": r.cfg.Password}
	b, _ := json.Marshal(payload)
	url := strings.TrimRight(r.cfg.BaseURL, "/") + "/auth/login"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	cli := &http.Client{Timeout: 30 * time.Second}
	res, err := cli.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("login failed: status=%d body=%s", res.StatusCode, string(raw))
	}
	var out struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.Data.Token == "" {
		return "", fmt.Errorf("empty token in login response")
	}
	return out.Data.Token, nil
}
