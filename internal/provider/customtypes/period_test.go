package customtypes

import (
	"context"
	"testing"
)

func TestParsePeriod(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"5 minutes", 300},
		{"1 minute", 60},
		{"1 hour 30 minutes", 5400},
		{"01:30:00", 5400},
		{"00:00:00", 0},
		{"1 week", 604800},
		{"7 days", 604800},
		{"1 month", 2592000},
		{"1 year", 31557600},
		{"2 days 3 hours", 183600},
		{"10 seconds", 10},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParsePeriod(tt.in)
			if err != nil {
				t.Fatalf("ParsePeriod(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParsePeriod(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

// A huge count must fail to parse, not wrap around to a small number.
func TestParsePeriodRejectsOverflow(t *testing.T) {
	if _, err := ParsePeriod("1537228672809129302 minutes"); err == nil {
		t.Error("an overflowing period parsed")
	}
	equal, _ := NewPeriodValue("1537228672809129302 minutes").StringSemanticEquals(context.Background(), NewPeriodValue("40 seconds"))
	if equal {
		t.Error("an overflowing period compared equal to 40 seconds")
	}
}

func TestParsePeriodRejectsOtherFormats(t *testing.T) {
	for _, in := range []string{"", "5m", "5 fortnights", "1:30:00", "one hour", "5 minutes ", "5  minutes"} {
		if _, err := ParsePeriod(in); err == nil {
			t.Errorf("ParsePeriod(%q) succeeded, want an error", in)
		}
	}
}

func TestPeriodSemanticEquality(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"1 week", "7 days", true},
		{"1 hour 30 minutes", "01:30:00", true},
		{"0 seconds", "00:00:00", true},
		{"1 day", "2 days", false},
		{"not a period", "1 day", false},
	}
	for _, tt := range tests {
		t.Run(tt.a+" vs "+tt.b, func(t *testing.T) {
			got, diags := NewPeriodValue(tt.a).StringSemanticEquals(context.Background(), NewPeriodValue(tt.b))
			if diags.HasError() {
				t.Fatalf("diagnostics: %v", diags)
			}
			if got != tt.want {
				t.Errorf("semantic equality = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPeriodTypeRoundTrip(t *testing.T) {
	ctx := context.Background()
	raw, err := NewPeriodValue("1 day").ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("ToTerraformValue: %v", err)
	}
	v, err := PeriodType{}.ValueFromTerraform(ctx, raw)
	if err != nil {
		t.Fatalf("ValueFromTerraform: %v", err)
	}
	pv, ok := v.(PeriodValue)
	if !ok || pv.ValueString() != "1 day" {
		t.Errorf("round trip = %#v, want PeriodValue \"1 day\"", v)
	}
	if !NewPeriodNull().IsNull() || !NewPeriodUnknown().IsUnknown() {
		t.Error("null and unknown constructors don't produce null and unknown values")
	}
}
