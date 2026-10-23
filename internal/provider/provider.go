// Package provider implements the Honeybadger Terraform provider on top of
// api-go's apiv3 client.
package provider

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/honeybadger-io/api-go/apiv3"
)

var _ provider.Provider = (*honeybadgerProvider)(nil)

type honeybadgerProvider struct {
	version string
}

// New returns a constructor for the provider, as providerserver expects.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &honeybadgerProvider{version: version}
	}
}

func (p *honeybadgerProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "honeybadger"
	resp.Version = p.version
}

func (p *honeybadgerProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Honeybadger configuration through Honeybadger's Data API (v3).",
		MarkdownDescription: "Manages Honeybadger configuration through Honeybadger's Data API (v3).\n\n" +
			"## API Token scopes\n\n" +
			"The API Token needs the scopes of the resources it manages. An account API Token holds the scopes chosen when it was created; a personal API Token acts as its user.\n\n" +
			generatedScopes,
		Attributes: map[string]schema.Attribute{
			"api_token": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				Description: "A Honeybadger API Token: an `hbu_` personal API Token, an `hba_` account API Token, " +
					"or an OAuth access token. Defaults to `HONEYBADGER_API_TOKEN`, then " +
					"`HONEYBADGER_PERSONAL_AUTH_TOKEN`.",
			},
			"api_url": schema.StringAttribute{
				Optional: true,
				Description: "The Data API host. Defaults to `HONEYBADGER_API_URL`, then " +
					"`https://app.honeybadger.io`. EU accounts use `https://eu-app.honeybadger.io`.",
			},
		},
	}
}

type providerModel struct {
	APIToken types.String `tfsdk:"api_token"`
	APIURL   types.String `tfsdk:"api_url"`
}

func (p *honeybadgerProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token, tokenSet := configuredString(resp, cfg.APIToken, "api_token", "HONEYBADGER_API_TOKEN")
	url, urlSet := configuredString(resp, cfg.APIURL, "api_url", "HONEYBADGER_API_URL")
	if resp.Diagnostics.HasError() {
		return
	}

	// Environment fallback applies only to attributes left out of config.
	if !tokenSet {
		token = firstSet(os.Getenv("HONEYBADGER_API_TOKEN"), os.Getenv("HONEYBADGER_PERSONAL_AUTH_TOKEN"))
	}
	if !urlSet {
		url = os.Getenv("HONEYBADGER_API_URL")
	}
	if token == "" {
		resp.Diagnostics.AddAttributeError(path.Root("api_token"), "Missing API token",
			"Set api_token, or the HONEYBADGER_API_TOKEN environment variable, to a Honeybadger API Token.")
		return
	}

	// Retries are api-go's (spec section 6.3). No http.Client with a Timeout:
	// it would cap every attempt, including long deletes.
	client := apiv3.NewClient().
		WithBearerToken(token).
		WithUserAgent("terraform-provider-honeybadger/" + p.version).
		WithRequestIDHook(logRequestID).
		WithRetry(apiv3.RetryPolicy{MaxAttempts: 4, MaxWait: time.Minute})
	if url != "" {
		client = client.WithBaseURL(url)
	}

	resp.ResourceData = client
	resp.DataSourceData = client
}

// configuredString returns an attribute's value and whether config set it. A
// value Terraform can't know yet (one computed from a resource still to be
// created) and an explicit empty string are both errors, so neither silently
// falls back to the environment or a default.
func configuredString(resp *provider.ConfigureResponse, v types.String, attr, envVar string) (string, bool) {
	switch {
	case v.IsUnknown():
		resp.Diagnostics.AddAttributeError(path.Root(attr), "Unknown provider configuration", fmt.Sprintf(
			"%s must be known when the provider is configured. Set it from a value that doesn't depend "+
				"on a resource still to be created, or use %s.", attr, envVar))
		return "", false
	case v.IsNull():
		return "", false
	case v.ValueString() == "":
		resp.Diagnostics.AddAttributeError(path.Root(attr), "Empty provider configuration", fmt.Sprintf(
			"%s is set to an empty string. Set a value, or remove it to use %s.", attr, envVar))
		return "", false
	default:
		return v.ValueString(), true
	}
}

// firstSet returns the first non-empty value.
func firstSet(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// logRequestID records the request id of each response that carries one, for
// TF_LOG=debug. It is best effort: responses without a body, such as a delete's
// 204, carry none.
func logRequestID(ctx context.Context, status int, requestID string) {
	tflog.Debug(ctx, "Honeybadger API response", map[string]any{
		"status":     status,
		"request_id": requestID,
	})
}

func (p *honeybadgerProvider) Resources(_ context.Context) []func() resource.Resource {
	return generatedResources()
}

func (p *honeybadgerProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}
