package provider

import (
	"testing"
	"time"

	"github.com/google/uuid"
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

func TestBoolPtr(t *testing.T) {
	if createBoolPtr(types.BoolNull()) != nil || *createBoolPtr(types.BoolValue(false)) {
		t.Error("createBoolPtr")
	}
	if updateBoolPtr(types.BoolValue(true), types.BoolValue(true)) != nil || !*updateBoolPtr(types.BoolValue(true), types.BoolValue(false)) {
		t.Error("updateBoolPtr")
	}
	if !boolPtrValue(nil).IsNull() {
		t.Error("boolPtrValue(nil)")
	}
}

func TestIsZero(t *testing.T) {
	if !isZero("") || isZero("x1") || !isZero(uuid.UUID{}) || isZero(uuid.MustParse("0b6f2b8e-4f7c-4c1a-9a51-6c1f0e1b2a3d")) {
		t.Error("isZero must be true only for \"\" and the zero UUID")
	}
}

func TestSetIntEnum(t *testing.T) {
	var s testSpeed
	setIntEnum(&s, types.Int64Value(5))
	setIntEnum(&s, types.Int64Null())
	if s != 5 {
		t.Errorf("s = %d, want 5 (a null leaves it unchanged)", s)
	}
}

func TestNullableTimeValue(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.FixedZone("x", 3600))
	if got := nullableTimeValue(nullable.NewNullableWithValue(at)); got.ValueString() != "2026-10-07T11:00:00Z" {
		t.Errorf("value = %v", got)
	}
	if got := nullableTimeValue(nullable.NewNullNullable[time.Time]()); !got.IsNull() {
		t.Errorf("null = %v", got)
	}
}
