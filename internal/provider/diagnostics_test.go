package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/honeybadger-io/api-go/apiv3"
)

// testSchema has a top-level, a nested, a list and a set attribute, which is
// every path shape a validation error can name.
var testSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"name": schema.StringAttribute{Required: true},
		"config": schema.SingleNestedAttribute{
			Optional:   true,
			Attributes: map[string]schema.Attribute{"url": schema.StringAttribute{Optional: true}},
		},
		"filters": schema.ListNestedAttribute{
			Optional: true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{"query": schema.StringAttribute{Optional: true}},
			},
		},
		"tags": schema.SetAttribute{Optional: true, ElementType: types.StringType},
	},
}

func apiErrorDiags(err error) diag.Diagnostics {
	var diags diag.Diagnostics
	addAPIError(context.Background(), &diags, testSchema, "Creating thing", err)
	return diags
}

func pathOf(d diag.Diagnostic) (path.Path, bool) {
	wp, ok := d.(diag.DiagnosticWithPath)
	if !ok {
		return path.Empty(), false
	}
	return wp.Path(), true
}

func TestValidationErrorsAttachToTheirAttributes(t *testing.T) {
	diags := apiErrorDiags(&apiv3.Error{
		StatusCode: 422,
		Code:       apiv3.CodeValidationError,
		Message:    "Validation failed",
		FieldErrors: []apiv3.FieldError{
			{Field: "name", Message: "can't be blank"},
			{Field: "config.url", Message: "is not a URL"},
			{Field: "filters.1.query", Message: "is invalid"},
		},
	})

	want := []path.Path{
		path.Root("name"),
		path.Root("config").AtName("url"),
		path.Root("filters").AtListIndex(1).AtName("query"),
	}
	if len(diags) != len(want) {
		t.Fatalf("got %d diagnostics, want %d: %v", len(diags), len(want), diags)
	}
	for i, d := range diags {
		p, ok := pathOf(d)
		if !ok || !p.Equal(want[i]) {
			t.Errorf("diagnostic %d path = %v (has path: %v), want %v", i, p, ok, want[i])
		}
		if d.Severity() != diag.SeverityError {
			t.Errorf("diagnostic %d severity = %v, want error", i, d.Severity())
		}
	}
}

// Review Focus 3: a field through a set, or one the schema doesn't have,
// becomes a general error that still names the field.
func TestValidationErrorOutsideTheSchemaIsGeneral(t *testing.T) {
	for _, field := range []string{"tags.0", "nonexistent", "config.nope"} {
		t.Run(field, func(t *testing.T) {
			diags := apiErrorDiags(&apiv3.Error{
				StatusCode:  422,
				Code:        apiv3.CodeValidationError,
				FieldErrors: []apiv3.FieldError{{Field: field, Message: "is wrong"}},
			})
			if len(diags) != 1 {
				t.Fatalf("got %d diagnostics, want 1", len(diags))
			}
			if _, ok := pathOf(diags[0]); ok {
				t.Errorf("diagnostic has a path, want a general error")
			}
			if !strings.Contains(diags[0].Detail(), field) {
				t.Errorf("detail %q doesn't name the field %q", diags[0].Detail(), field)
			}
		})
	}
}

func TestErrorCodesBecomeSpecificDiagnostics(t *testing.T) {
	reset := time.Date(2026, 10, 5, 14, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		err        *apiv3.Error
		wantDetail string
	}{
		{"insufficient scope names the scope",
			&apiv3.Error{StatusCode: 403, Code: apiv3.CodeInsufficientScope,
				Details: map[string]any{"required_scope": "check_ins:write"}},
			"check_ins:write"},
		{"plan limit says it's the plan",
			&apiv3.Error{StatusCode: 403, Code: apiv3.CodeLimitReached, Message: "Alarm limit reached"},
			"plan"},
		{"feature unavailable says it's the plan",
			&apiv3.Error{StatusCode: 403, Code: apiv3.CodeFeatureUnavailable, Message: "Custom retention"},
			"plan"},
		{"401 points at the token",
			&apiv3.Error{StatusCode: 401, Message: "Unauthorized"},
			"api_token"},
		{"access denied points at the token",
			&apiv3.Error{StatusCode: 403, Code: apiv3.CodeAccessDenied, Message: "Access denied"},
			"api_token"},
		{"ambiguous account asks for a single-account token",
			&apiv3.Error{StatusCode: 422, Code: apiv3.CodeAmbiguousAccount},
			"single account"},
		{"rate limit gives the reset time",
			&apiv3.Error{StatusCode: 429, Code: apiv3.CodeRateLimitExceeded,
				RateLimit: &apiv3.RateLimit{Limit: 360, Reset: reset}},
			"2026-10-05T14:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := apiErrorDiags(tt.err)
			if len(diags) != 1 || !diags.HasError() {
				t.Fatalf("got %v, want one error", diags)
			}
			if !strings.Contains(diags[0].Detail(), tt.wantDetail) {
				t.Errorf("detail %q doesn't contain %q", diags[0].Detail(), tt.wantDetail)
			}
		})
	}
}

func TestRequestIDIsIncludedWhenPresent(t *testing.T) {
	with := apiErrorDiags(&apiv3.Error{StatusCode: 500, Message: "boom", RequestID: "req_42"})
	if !strings.Contains(with[0].Detail(), "req_42") {
		t.Errorf("detail %q doesn't include the request id", with[0].Detail())
	}

	without := apiErrorDiags(&apiv3.Error{StatusCode: 500, Message: "boom"})
	if strings.Contains(without[0].Detail(), "Request ID") {
		t.Errorf("detail %q mentions a request id the error didn't have", without[0].Detail())
	}
}

// Review Focus 5: a body that isn't the JSON envelope still reads well.
func TestNonEnvelopeErrorIsReadable(t *testing.T) {
	diags := apiErrorDiags(&apiv3.Error{StatusCode: 502, Message: "502 Bad Gateway", Body: []byte("<html>Bad Gateway</html>")})
	if len(diags) != 1 || !strings.Contains(diags[0].Detail(), "502") {
		t.Errorf("got %v, want one error mentioning the status", diags)
	}
}

func TestNonAPIErrorIsGeneral(t *testing.T) {
	diags := apiErrorDiags(errors.New("dial tcp: connection refused"))
	if len(diags) != 1 || !strings.Contains(diags[0].Detail(), "connection refused") {
		t.Errorf("got %v, want one error carrying the cause", diags)
	}
}

func TestIsUnknownOutcome(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"no response", errors.New("connection reset by peer"), true},
		{"wrapped no response", fmt.Errorf("creating: %w", errors.New("EOF")), true},
		{"503", &apiv3.Error{StatusCode: 503, Code: apiv3.CodeServiceUnavailable}, true},
		{"502 from a load balancer", &apiv3.Error{StatusCode: 502}, true},
		{"504 from a load balancer", &apiv3.Error{StatusCode: 504}, true},
		{"422", &apiv3.Error{StatusCode: 422, Code: apiv3.CodeValidationError}, false},
		{"404", &apiv3.Error{StatusCode: 404, Code: apiv3.CodeNotFound}, false},
		{"429", &apiv3.Error{StatusCode: 429, Code: apiv3.CodeRateLimitExceeded}, false},
		// Never sent, so nothing was created.
		{"cancelled", fmt.Errorf("creating: %w", context.Canceled), false},
		{"DNS failure", &net.DNSError{Err: "no such host", Name: "nope.invalid"}, false},
		{"connection refused", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUnknownOutcome(tt.err); got != tt.want {
				t.Errorf("isUnknownOutcome = %v, want %v", got, tt.want)
			}
		})
	}
}

// A 2xx whose body was cut off means the write happened but its result is
// unknown. apiv3 reports it as an *Error wrapping the read failure.
func TestIsUnknownOutcomeForTruncatedSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"p1"`))
	}))
	t.Cleanup(srv.Close)

	client := apiv3.NewClient().WithBaseURL(srv.URL).WithBearerToken("hbt_test")
	_, err := client.Projects.Get(context.Background(), "p1")
	if err == nil {
		t.Fatal("Get succeeded on a truncated body")
	}
	if !isUnknownOutcome(err) {
		t.Errorf("isUnknownOutcome(%v) = false, want true", err)
	}
}

// A cancellation while a 2xx body is being read doesn't make the outcome
// known: the server had already accepted the write.
func TestIsUnknownOutcomeForCancelledSuccessBody(t *testing.T) {
	flushed := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"p1"`))
		w.(http.Flusher).Flush()
		close(flushed)
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		<-flushed
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	client := apiv3.NewClient().WithBaseURL(srv.URL).WithBearerToken("hbt_test")
	_, err := client.Projects.Get(ctx, "p1")
	if err == nil {
		t.Fatal("Get succeeded on a cancelled body read")
	}
	if !isUnknownOutcome(err) {
		t.Errorf("isUnknownOutcome(%v) = false, want true", err)
	}
}

// A 2xx with a body that isn't usable (malformed JSON, or no data) also means
// the write happened but no id came back.
func TestIsUnknownOutcomeForUnusableSuccess(t *testing.T) {
	for name, body := range map[string]string{
		"malformed": `not json`,
		"no data":   `{"meta":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(body))
			}))
			t.Cleanup(srv.Close)

			client := apiv3.NewClient().WithBaseURL(srv.URL).WithBearerToken("hbt_test")
			_, err := client.Projects.Get(context.Background(), "p1")
			if err == nil {
				t.Fatalf("Get succeeded on a %s body", name)
			}
			if !isUnknownOutcome(err) {
				t.Errorf("isUnknownOutcome(%v) = false, want true", err)
			}
		})
	}
}

func TestUnknownCreateOutcomeTellsHowToAdopt(t *testing.T) {
	cause := errors.New("connection reset by peer")

	t.Run("named", func(t *testing.T) {
		var diags diag.Diagnostics
		addUnknownCreateOutcome(&diags, createOutcome{
			TypeName: "honeybadger_check_in", ProjectID: "Xk9mZp", Name: "nightly",
			ImportFormat: "<project_id>/<id>", FindHint: "the project's check-ins",
		}, cause)
		d := diags[0].Detail()
		for _, want := range []string{"nightly", "Xk9mZp", "terraform import", "<project_id>/<id>", "connection reset"} {
			if !strings.Contains(d, want) {
				t.Errorf("detail %q doesn't contain %q", d, want)
			}
		}
	})

	t.Run("unnamed", func(t *testing.T) {
		var diags diag.Diagnostics
		addUnknownCreateOutcome(&diags, createOutcome{
			TypeName: "honeybadger_project_key", ProjectID: "Xk9mZp",
			ImportFormat: "<project_id>/<id>", FindHint: "the project's API keys",
		}, cause)
		d := diags[0].Detail()
		for _, want := range []string{"the project's API keys", "terraform import", "<project_id>/<id>"} {
			if !strings.Contains(d, want) {
				t.Errorf("detail %q doesn't contain %q", d, want)
			}
		}
		if strings.Contains(d, `named ""`) {
			t.Errorf("detail %q quotes an empty name", d)
		}
	})
}

func TestUnknownDeleteOutcomeSaysToRunAgain(t *testing.T) {
	var diags diag.Diagnostics
	addUnknownDeleteOutcome(&diags, "honeybadger_project", "Xk9mZp", errors.New("context deadline exceeded"))
	d := diags[0].Detail()
	for _, want := range []string{"Xk9mZp", "terraform destroy"} {
		if !strings.Contains(d, want) {
			t.Errorf("detail %q doesn't contain %q", d, want)
		}
	}
}
