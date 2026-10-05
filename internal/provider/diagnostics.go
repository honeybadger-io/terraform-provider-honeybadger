package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/honeybadger-io/api-go/apiv3"
)

// attributeTypes is the part of a resource or data source schema that error
// mapping needs: whether a path exists in it.
type attributeTypes interface {
	TypeAtPath(context.Context, path.Path) (attr.Type, diag.Diagnostics)
}

// addAPIError turns an error from an apiv3 call into diagnostics. summary says
// what failed, such as "Creating check-in".
func addAPIError(ctx context.Context, diags *diag.Diagnostics, schema attributeTypes, summary string, err error) {
	var apiErr *apiv3.Error
	if !errors.As(err, &apiErr) {
		diags.AddError(summary, err.Error())
		return
	}

	detail := func(msg string) string {
		if apiErr.RequestID == "" {
			return msg
		}
		return msg + "\n\nRequest ID: " + apiErr.RequestID
	}

	switch {
	case apiErr.Code == apiv3.CodeValidationError && len(apiErr.FieldErrors) > 0:
		for _, fe := range apiErr.FieldErrors {
			if p, ok := fieldPath(ctx, schema, fe.Field); ok {
				diags.AddAttributeError(p, summary, detail(fe.Message))
			} else {
				diags.AddError(summary, detail(fe.Field+": "+fe.Message))
			}
		}
	case apiErr.Code == apiv3.CodeInsufficientScope:
		scope := apiErr.RequiredScope()
		if scope == "" {
			scope = "required"
		}
		diags.AddError(summary, detail(fmt.Sprintf(
			"The API token is missing the %q scope. Use a token that has it.", scope)))
	case apiErr.Code == apiv3.CodeFeatureUnavailable, apiErr.Code == apiv3.CodeLimitReached:
		diags.AddError(summary, detail("The Honeybadger account's plan doesn't allow this: "+apiErr.Message))
	case apiErr.StatusCode == http.StatusUnauthorized,
		apiErr.Code == apiv3.CodeUnauthorized,
		apiErr.Code == apiv3.CodeAccessDenied:
		diags.AddError(summary, detail("The API token was refused: "+apiErr.Message+
			"\n\nCheck the provider's api_token, or the HONEYBADGER_API_TOKEN environment variable."))
	case apiErr.Code == apiv3.CodeAmbiguousAccount:
		diags.AddError(summary, detail(
			"The API token covers more than one Honeybadger account. Use a token for a single account."))
	case apiErr.Code == apiv3.CodeRateLimitExceeded:
		msg := "The Honeybadger API rate limit was reached."
		if apiErr.RateLimit != nil {
			msg += " It resets at " + apiErr.RateLimit.Reset.UTC().Format(time.RFC3339) + "."
		}
		diags.AddError(summary, detail(msg))
	default:
		diags.AddError(summary, detail(apiErr.Error()))
	}
}

// fieldPath turns a validation error's dotted field ("config.url",
// "filters.1.query") into a schema path. ok is false when the schema has no
// such path, including any path through a set, whose elements have no index.
func fieldPath(ctx context.Context, schema attributeTypes, field string) (path.Path, bool) {
	if field == "" {
		return path.Empty(), false
	}
	parts := strings.Split(field, ".")
	p := path.Root(parts[0])
	for _, part := range parts[1:] {
		if i, err := strconv.Atoi(part); err == nil {
			p = p.AtListIndex(i)
		} else {
			p = p.AtName(part)
		}
	}
	if _, d := schema.TypeAtPath(ctx, p); d.HasError() {
		return path.Empty(), false
	}
	return p, true
}

// isUnknownOutcome reports whether err leaves it unknown whether a write took
// effect (spec section 6.4): the request may have reached the server, but no
// usable answer came back. That covers a dropped connection or timeout, a 503,
// a 502 or 504 from a load balancer, and a 2xx whose body couldn't be read or
// decoded. A request that was never sent has a known outcome: nothing changed.
func isUnknownOutcome(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var apiErr *apiv3.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
		return apiErr.StatusCode >= 200 && apiErr.StatusCode < 300
	}
	return !neverSent(err)
}

// neverSent reports errors raised before the request reached the server: the
// host didn't resolve, or the connection was never made.
func neverSent(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}

// createOutcome describes a create whose result is unknown, for the error that
// tells the user how to find and adopt the object (spec section 6.4).
type createOutcome struct {
	TypeName     string // e.g. "honeybadger_check_in"
	ProjectID    string // empty for account-level resources
	Name         string // configured name or label; empty when there is none
	ImportFormat string // e.g. "<project_id>/<id>"
	FindHint     string // where to look, e.g. "the project's check-ins"
}

func addUnknownCreateOutcome(diags *diag.Diagnostics, o createOutcome, err error) {
	var b strings.Builder
	fmt.Fprintf(&b, "The request failed (%v), and it may have created the %s anyway.\n\n", err, o.TypeName)
	if o.Name != "" {
		fmt.Fprintf(&b, "Look for one named %q in %s", o.Name, o.FindHint)
	} else {
		fmt.Fprintf(&b, "Look for a new one in %s", o.FindHint)
	}
	if o.ProjectID != "" {
		fmt.Fprintf(&b, " (project %s)", o.ProjectID)
	}
	fmt.Fprintf(&b, ". If it exists, adopt it instead of creating another:\n\n"+
		"  terraform import <resource address> %s\n\n"+
		"If it doesn't, run terraform apply again.", o.ImportFormat)
	diags.AddError("Create may have succeeded", b.String())
}

func addUnknownDeleteOutcome(diags *diag.Diagnostics, typeName, id string, err error) {
	diags.AddError("Delete may still be in progress", fmt.Sprintf(
		"Deleting %s %s didn't finish (%v), and Honeybadger may still be deleting it. "+
			"Run terraform destroy again; an object that's already gone counts as deleted.",
		typeName, id, err))
}
