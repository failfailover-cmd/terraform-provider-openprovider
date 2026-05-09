package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &domainNameserversResource{}
var _ resource.ResourceWithImportState = &domainNameserversResource{}

type domainNameserversResource struct{ cfg *providerConfig }

type domainNameserversModel struct {
	ID          types.String `tfsdk:"id"`
	Domain      types.String `tfsdk:"domain"`
	Nameservers types.Set    `tfsdk:"nameservers"`
}

func NewDomainNameserversResource() resource.Resource { return &domainNameserversResource{} }

func (r *domainNameserversResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain_nameservers"
}

func (r *domainNameserversResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Attributes: map[string]schema.Attribute{
		"id":          schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"domain":      schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"nameservers": schema.SetAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()}},
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

	token, err := r.login(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Openprovider API error", err.Error())
		return
	}

	ns, status, err := r.fetchNS(ctx, token, st.Domain.ValueString())
	if err != nil {
		if status == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Openprovider API error", err.Error())
		return
	}

	st.ID = st.Domain
	st.Nameservers, _ = types.SetValueFrom(ctx, types.StringType, ns)
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
	domainID, sld, tld, err := r.resolveDomainID(ctx, token, plan.Domain.ValueString())
	if err != nil {
		return err
	}
	var ns []string
	if diags := plan.Nameservers.ElementsAs(ctx, &ns, false); diags.HasError() {
		return fmt.Errorf("invalid nameservers list")
	}
	if len(ns) == 0 {
		return fmt.Errorf("nameservers list cannot be empty")
	}

	type nsObj struct {
		Name string `json:"name"`
	}
	list := make([]nsObj, 0, len(ns))
	for _, n := range ns {
		list = append(list, nsObj{Name: strings.TrimSpace(n)})
	}
	body := map[string]any{
		"domain": map[string]string{
			"name":      sld,
			"extension": tld,
		},
		"name_servers": list,
		"remove_nses":  true,
	}
	b, _ := json.Marshal(body)

	url := strings.TrimRight(r.cfg.BaseURL, "/") + "/domains/" + domainID
	h := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}
	status, raw, err := r.doJSON(ctx, http.MethodPut, url, h, b)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("PUT /domains failed: status=%d body=%s", status, raw)
	}
	return nil
}

func (r *domainNameserversResource) fetchNS(ctx context.Context, token, domain string) ([]string, int, error) {
	domainID, _, _, err := r.resolveDomainID(ctx, token, domain)
	if err != nil {
		return nil, 0, err
	}
	url := strings.TrimRight(r.cfg.BaseURL, "/") + "/domains/" + domainID
	h := map[string]string{"Authorization": "Bearer " + token}
	status, raw, err := r.doJSON(ctx, http.MethodGet, url, h, nil)
	if err != nil {
		return nil, status, err
	}
	if status == http.StatusNotFound {
		return nil, status, fmt.Errorf("domain not found")
	}
	if status < 200 || status >= 300 {
		return nil, status, fmt.Errorf("GET /domains failed: status=%d body=%s", status, raw)
	}

	var out struct {
		Data struct {
			NameServers []struct {
				Name string `json:"name"`
			} `json:"name_servers"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, status, fmt.Errorf("decode domain payload: %w", err)
	}

	ns := make([]string, 0, len(out.Data.NameServers))
	for _, n := range out.Data.NameServers {
		if strings.TrimSpace(n.Name) != "" {
			ns = append(ns, strings.ToLower(strings.TrimSpace(n.Name)))
		}
	}
	sort.Strings(ns)
	return ns, status, nil
}

func (r *domainNameserversResource) resolveDomainID(ctx context.Context, token, domain string) (string, string, string, error) {
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("invalid domain: %s", domain)
	}
	sld := parts[0]
	tld := strings.Join(parts[1:], ".")

	q := strings.TrimRight(r.cfg.BaseURL, "/") + "/domains?full_name=" + domain + "&limit=1&offset=0"
	h := map[string]string{"Authorization": "Bearer " + token}
	status, raw, err := r.doJSON(ctx, http.MethodGet, q, h, nil)
	if err != nil {
		return "", "", "", err
	}
	if status < 200 || status >= 300 {
		return "", "", "", fmt.Errorf("GET /domains search failed: status=%d body=%s", status, raw)
	}

	var out struct {
		Data struct {
			Results []struct {
				ID       int64  `json:"id"`
				FullName string `json:"full_name"`
				Domain   struct {
					Name      string `json:"name"`
					Extension string `json:"extension"`
				} `json:"domain"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return "", "", "", fmt.Errorf("decode domains search payload: %w", err)
	}
	if len(out.Data.Results) == 0 {
		return "", "", "", fmt.Errorf("domain not found in openprovider: %s", domain)
	}

	row := out.Data.Results[0]
	id := strconv.FormatInt(row.ID, 10)
	if id == "0" {
		return "", "", "", fmt.Errorf("invalid domain id for %s", domain)
	}
	if row.Domain.Name != "" && row.Domain.Extension != "" {
		return id, row.Domain.Name, row.Domain.Extension, nil
	}
	return id, sld, tld, nil
}

func (r *domainNameserversResource) login(ctx context.Context) (string, error) {
	payload := map[string]string{"username": r.cfg.Username, "password": r.cfg.Password}
	b, _ := json.Marshal(payload)
	url := strings.TrimRight(r.cfg.BaseURL, "/") + "/auth/login"
	h := map[string]string{"Content-Type": "application/json"}
	status, raw, err := r.doJSON(ctx, http.MethodPost, url, h, b)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("login failed: status=%d body=%s", status, raw)
	}
	var out struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return "", err
	}
	if out.Data.Token == "" {
		return "", fmt.Errorf("empty token in login response")
	}
	return out.Data.Token, nil
}

func (r *domainNameserversResource) doJSON(ctx context.Context, method, url string, headers map[string]string, body []byte) (int, string, error) {
	var lastErr error
	for attempt := 0; attempt <= r.cfg.MaxRetries; attempt++ {
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		req, _ := http.NewRequestWithContext(ctx, method, url, reader)
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		cli := &http.Client{Timeout: r.cfg.RequestTimeout}
		res, err := cli.Do(req)
		if err != nil {
			lastErr = err
			if !isRetryableNetErr(err) || attempt == r.cfg.MaxRetries {
				return 0, "", err
			}
			time.Sleep(r.backoff(attempt, ""))
			continue
		}

		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		bodyStr := string(raw)

		if retryableStatus(res.StatusCode) && attempt < r.cfg.MaxRetries {
			time.Sleep(r.backoff(attempt, res.Header.Get("Retry-After")))
			continue
		}

		if res.StatusCode >= 200 && res.StatusCode < 300 {
			return res.StatusCode, bodyStr, nil
		}
		return res.StatusCode, bodyStr, nil
	}
	return 0, "", fmt.Errorf("request retries exhausted: %w", lastErr)
}

func retryableStatus(code int) bool {
	if code == http.StatusTooManyRequests || code == 1015 {
		return true
	}
	return code >= 500 && code <= 599
}

func isRetryableNetErr(err error) bool {
	if nerr, ok := err.(net.Error); ok {
		return nerr.Timeout() || nerr.Temporary()
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "connection reset") || strings.Contains(msg, "broken pipe")
}

func (r *domainNameserversResource) backoff(attempt int, retryAfter string) time.Duration {
	if retryAfter != "" {
		if sec, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && sec > 0 {
			d := time.Duration(sec) * time.Second
			if d > r.cfg.MaxBackoff {
				return r.cfg.MaxBackoff
			}
			return d
		}
	}
	d := r.cfg.BaseBackoff * (1 << attempt)
	if d > r.cfg.MaxBackoff {
		return r.cfg.MaxBackoff
	}
	return d
}
