package provider

import (
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
)

type kind string

func TestStringPointers(t *testing.T) {
	if p := createStringPtr(types.StringNull()); p != nil {
		t.Errorf("createStringPtr(null) = %q, want nil", *p)
	}
	if p := createStringPtr(types.StringUnknown()); p != nil {
		t.Errorf("createStringPtr(unknown) = %q, want nil", *p)
	}
	if p := createStringPtr(types.StringValue("x")); p == nil || *p != "x" {
		t.Errorf("createStringPtr(x) = %v, want x", p)
	}
	if p := updateStringPtr(types.StringValue("same"), types.StringValue("same")); p != nil {
		t.Errorf("updateStringPtr(unchanged) = %q, want nil", *p)
	}
	if p := updateStringPtr(types.StringValue("new"), types.StringValue("old")); p == nil || *p != "new" {
		t.Errorf("updateStringPtr(changed) = %v, want new", p)
	}
	if v := stringPtrValue(nil); !v.IsNull() {
		t.Errorf("stringPtrValue(nil) = %v, want null", v)
	}
	s := "x"
	if v := stringPtrValue(&s); !v.Equal(types.StringValue("x")) {
		t.Errorf("stringPtrValue(&x) = %v", v)
	}
}

func TestEnums(t *testing.T) {
	var k kind
	setEnum(&k, types.StringValue("cron"))
	if k != "cron" {
		t.Errorf("setEnum = %q, want cron", k)
	}
	setEnum(&k, types.StringNull())
	if k != "cron" {
		t.Errorf("setEnum(null) changed the value to %q", k)
	}

	var p *kind
	setEnumPtr(&p, types.StringValue("simple"))
	if p == nil || *p != "simple" {
		t.Errorf("setEnumPtr = %v, want simple", p)
	}

	var u *kind
	updateEnumPtr(&u, types.StringValue("cron"), types.StringValue("cron"))
	if u != nil {
		t.Errorf("updateEnumPtr(unchanged) = %q, want nil", *u)
	}
	updateEnumPtr(&u, types.StringValue("cron"), types.StringValue("simple"))
	if u == nil || *u != "cron" {
		t.Errorf("updateEnumPtr(changed) = %v, want cron", u)
	}
}

func TestPeriodAndTimeValues(t *testing.T) {
	if v := periodValue(nullable.NewNullNullable[string]()); !v.IsNull() {
		t.Errorf("periodValue(null) = %v, want null", v)
	}
	if v := periodValue(nullable.NewNullableWithValue("7 days")); v.ValueString() != "7 days" {
		t.Errorf("periodValue = %v, want 7 days", v)
	}
	at := time.Date(2026, 10, 5, 9, 30, 0, 0, time.FixedZone("x", -7*3600))
	if v := timeValue(at); v.ValueString() != "2026-10-05T16:30:00Z" {
		t.Errorf("timeValue = %v, want UTC RFC 3339", v)
	}
}

func TestErrNoResultIsAnUnknownOutcome(t *testing.T) {
	if !isUnknownOutcome(errNoResult) {
		t.Error("a create answered without a usable result must be an unknown outcome")
	}
}
