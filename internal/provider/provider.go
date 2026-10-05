// Package provider implements the Honeybadger Terraform provider on top of
// api-go's apiv3 client.
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
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
		Description: "Manages Honeybadger configuration through the v3 API.",
		Attributes: map[string]schema.Attribute{
			"api_token": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				Description: "A Honeybadger API token: an `hbt_` personal token, an `hba_` account token, " +
					"or an OAuth access token. Defaults to `HONEYBADGER_API_TOKEN`, then " +
					"`HONEYBADGER_PERSONAL_AUTH_TOKEN`.",
			},
			"api_url": schema.StringAttribute{
				Optional: true,
				Description: "The Honeybadger API host. Defaults to `HONEYBADGER_API_URL`, then " +
					"`https://app.honeybadger.io`. EU accounts use `https://eu-app.honeybadger.io`.",
			},
		},
	}
}

func (p *honeybadgerProvider) Configure(_ context.Context, _ provider.ConfigureRequest, _ *provider.ConfigureResponse) {
}

func (p *honeybadgerProvider) Resources(_ context.Context) []func() resource.Resource {
	return nil
}

func (p *honeybadgerProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}
