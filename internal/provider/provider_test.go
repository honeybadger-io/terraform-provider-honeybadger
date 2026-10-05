package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
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
