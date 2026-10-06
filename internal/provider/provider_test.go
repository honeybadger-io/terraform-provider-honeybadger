package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/honeybadger-io/api-go/apiv3"
)

func TestProviderSchema(t *testing.T) {
	server, err := providerserver.NewProtocol6WithError(New("test")())()
	if err != nil {
		t.Fatalf("NewProtocol6WithError: %v", err)
	}

	resp, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("GetProviderSchema: %v", err)
	}
	for _, d := range resp.Diagnostics {
		t.Errorf("diagnostic: %s: %s", d.Summary, d.Detail)
	}

	attrs := map[string]*tfprotov6.SchemaAttribute{}
	for _, a := range resp.Provider.Block.Attributes {
		attrs[a.Name] = a
	}

	token, ok := attrs["api_token"]
	if !ok {
		t.Fatal("api_token is missing from the provider schema")
	}
	if !token.Optional || token.Required || !token.Sensitive {
		t.Errorf("api_token: optional=%v required=%v sensitive=%v, want optional and sensitive",
			token.Optional, token.Required, token.Sensitive)
	}

	url, ok := attrs["api_url"]
	if !ok {
		t.Fatal("api_url is missing from the provider schema")
	}
	if !url.Optional || url.Required || url.Sensitive {
		t.Errorf("api_url: optional=%v required=%v sensitive=%v, want optional only",
			url.Optional, url.Required, url.Sensitive)
	}

	if len(attrs) != 2 {
		t.Errorf("provider schema has %d attributes, want exactly api_token and api_url", len(attrs))
	}
}

var (
	nullString    = tftypes.NewValue(tftypes.String, nil)
	unknownString = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
)

func stringValueOf(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }

// clearEnv keeps the developer's own Honeybadger variables out of a test.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"HONEYBADGER_API_TOKEN", "HONEYBADGER_PERSONAL_AUTH_TOKEN", "HONEYBADGER_API_URL"} {
		t.Setenv(name, "")
	}
}

// configure runs the provider's Configure with the given attribute values.
func configure(t *testing.T, token, url tftypes.Value) *provider.ConfigureResponse {
	t.Helper()
	ctx := context.Background()
	p := New("test")()

	var schemaResp provider.SchemaResponse
	p.Schema(ctx, provider.SchemaRequest{}, &schemaResp)

	raw := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), map[string]tftypes.Value{
		"api_token": token,
		"api_url":   url,
	})
	resp := &provider.ConfigureResponse{}
	p.Configure(ctx, provider.ConfigureRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)
	return resp
}

// seen records what the configured client sent.
type seen struct {
	mu    sync.Mutex
	auth  []string
	paths []string
}

// apiServer answers project reads and records each request.
func apiServer(t *testing.T) (*httptest.Server, *seen) {
	t.Helper()
	s := &seen{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.auth = append(s.auth, r.Header.Get("Authorization"))
		s.paths = append(s.paths, r.URL.Path)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"p1","name":"Project"}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, s
}

// getProject makes one call with the client Configure produced.
func getProject(t *testing.T, resp *provider.ConfigureResponse) {
	t.Helper()
	client, ok := resp.ResourceData.(*apiv3.Client)
	if !ok {
		t.Fatalf("ResourceData = %T, want *apiv3.Client", resp.ResourceData)
	}
	if resp.DataSourceData != resp.ResourceData {
		t.Error("DataSourceData isn't the same client as ResourceData")
	}
	if _, err := client.Projects.Get(context.Background(), "p1"); err != nil {
		t.Fatalf("Projects.Get: %v", err)
	}
}

func assertNoDiags(t *testing.T, diags diag.Diagnostics) {
	t.Helper()
	for _, d := range diags {
		t.Errorf("unexpected diagnostic: %s: %s", d.Summary(), d.Detail())
	}
}

func assertAttrError(t *testing.T, diags diag.Diagnostics, attr string) {
	t.Helper()
	for _, d := range diags {
		if wp, ok := d.(diag.DiagnosticWithPath); ok && wp.Path().Equal(path.Root(attr)) && d.Severity() == diag.SeverityError {
			return
		}
	}
	t.Errorf("no error on %s in %v", attr, diags)
}

func TestConfigureUsesConfiguredTokenAndURL(t *testing.T) {
	clearEnv(t)
	srv, s := apiServer(t)

	resp := configure(t, stringValueOf("hbt_config"), stringValueOf(srv.URL))
	assertNoDiags(t, resp.Diagnostics)
	getProject(t, resp)

	if s.auth[0] != "Bearer hbt_config" {
		t.Errorf("Authorization = %q, want the configured token", s.auth[0])
	}
	if s.paths[0] != "/v3/projects/p1" {
		t.Errorf("path = %q, want /v3/projects/p1", s.paths[0])
	}
}

// Review Focus 1: a trailing slash or an explicit /v3 still reaches /v3 once.
func TestConfigureAcceptsURLWithVersionSegment(t *testing.T) {
	clearEnv(t)
	srv, s := apiServer(t)

	resp := configure(t, stringValueOf("hbt_config"), stringValueOf(srv.URL+"/v3/"))
	assertNoDiags(t, resp.Diagnostics)
	getProject(t, resp)

	if s.paths[0] != "/v3/projects/p1" {
		t.Errorf("path = %q, want /v3/projects/p1", s.paths[0])
	}
}

func TestConfigureFallsBackToEnvironment(t *testing.T) {
	clearEnv(t)
	srv, s := apiServer(t)
	t.Setenv("HONEYBADGER_API_TOKEN", "hba_env")
	t.Setenv("HONEYBADGER_API_URL", srv.URL)

	resp := configure(t, nullString, nullString)
	assertNoDiags(t, resp.Diagnostics)
	getProject(t, resp)

	if s.auth[0] != "Bearer hba_env" {
		t.Errorf("Authorization = %q, want the env token", s.auth[0])
	}
}

func TestConfigurePrefersAPITokenOverPersonalAuthToken(t *testing.T) {
	clearEnv(t)
	srv, s := apiServer(t)
	t.Setenv("HONEYBADGER_API_TOKEN", "hba_new")
	t.Setenv("HONEYBADGER_PERSONAL_AUTH_TOKEN", "hbt_old")

	resp := configure(t, nullString, stringValueOf(srv.URL))
	assertNoDiags(t, resp.Diagnostics)
	getProject(t, resp)

	if s.auth[0] != "Bearer hba_new" {
		t.Errorf("Authorization = %q, want HONEYBADGER_API_TOKEN's value", s.auth[0])
	}
}

func TestConfigureAcceptsPersonalAuthToken(t *testing.T) {
	clearEnv(t)
	srv, s := apiServer(t)
	t.Setenv("HONEYBADGER_PERSONAL_AUTH_TOKEN", "hbt_mcp")

	resp := configure(t, nullString, stringValueOf(srv.URL))
	assertNoDiags(t, resp.Diagnostics)
	getProject(t, resp)

	if s.auth[0] != "Bearer hbt_mcp" {
		t.Errorf("Authorization = %q, want the MCP server's token variable", s.auth[0])
	}
}

func TestConfigureRequiresToken(t *testing.T) {
	clearEnv(t)

	resp := configure(t, nullString, nullString)
	assertAttrError(t, resp.Diagnostics, "api_token")
	if resp.ResourceData != nil {
		t.Error("a client was configured without a token")
	}
}

// Review Focus 2 and 4: an empty string in config is a mistake, not "unset".
// Environment fallback applies only to null attributes, so an empty api_url
// must not quietly become production, and an empty api_token must not become
// an empty Bearer header or pick up a token from the environment.
func TestConfigureRejectsEmptyValues(t *testing.T) {
	clearEnv(t)
	t.Setenv("HONEYBADGER_API_TOKEN", "hba_env")
	t.Setenv("HONEYBADGER_API_URL", "http://localhost:3001")

	resp := configure(t, stringValueOf(""), nullString)
	assertAttrError(t, resp.Diagnostics, "api_token")
	if resp.ResourceData != nil {
		t.Error("a client was configured from an empty token")
	}

	resp = configure(t, stringValueOf("hbt_config"), stringValueOf(""))
	assertAttrError(t, resp.Diagnostics, "api_url")
	if resp.ResourceData != nil {
		t.Error("a client was configured from an empty URL")
	}
}

func TestConfigureRejectsUnknownValues(t *testing.T) {
	clearEnv(t)
	t.Setenv("HONEYBADGER_API_TOKEN", "hba_env")

	resp := configure(t, unknownString, nullString)
	assertAttrError(t, resp.Diagnostics, "api_token")
	if resp.ResourceData != nil {
		t.Error("a client was configured from an unknown token")
	}

	resp = configure(t, stringValueOf("hbt_config"), unknownString)
	assertAttrError(t, resp.Diagnostics, "api_url")
	if resp.ResourceData != nil {
		t.Error("a client was configured from an unknown URL")
	}
}

func TestConfiguredClientRetriesRateLimits(t *testing.T) {
	clearEnv(t)
	var calls int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"p1","name":"Project"}}`))
	}))
	t.Cleanup(srv.Close)

	resp := configure(t, stringValueOf("hbt_config"), stringValueOf(srv.URL))
	assertNoDiags(t, resp.Diagnostics)
	getProject(t, resp)

	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Errorf("requests = %d, want 2 (one 429, one retry)", calls)
	}
}

func TestProviderRegistersTheResources(t *testing.T) {
	ctx := context.Background()
	var names []string
	for _, f := range New("test")().Resources(ctx) {
		resp := &resource.MetadataResponse{}
		f().Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "honeybadger"}, resp)
		names = append(names, resp.TypeName)
	}
	sort.Strings(names)
	want := []string{"honeybadger_alarm", "honeybadger_check_in", "honeybadger_project", "honeybadger_project_key"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("resources = %v, want %v", names, want)
	}
}
