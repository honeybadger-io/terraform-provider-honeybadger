// Package customtypes holds Terraform value types with semantic equality.
package customtypes

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// periodFormat is the API's accepted check-in period format: unit sums such as
// "1 hour 30 minutes", or HH:MM:SS.
var periodFormat = regexp.MustCompile(
	`^(\d+ (second|minute|hour|day|week|month|year)s?)( \d+ (second|minute|hour|day|week|month|year)s?)*$|^\d\d:\d\d:\d\d$`)

// unitSeconds matches ChronicDuration 0.10.6, which the API parses periods
// with: a month is 30 days and a year is 365.25 days.
var unitSeconds = map[string]int64{
	"second": 1,
	"minute": 60,
	"hour":   3600,
	"day":    86400,
	"week":   604800,
	"month":  2592000,
	"year":   31557600,
}

// ParsePeriod returns a period's length in seconds.
func ParsePeriod(s string) (int64, error) {
	if !periodFormat.MatchString(s) {
		return 0, fmt.Errorf("%q isn't a period like \"5 minutes\", \"1 hour 30 minutes\" or \"01:30:00\"", s)
	}
	if strings.Contains(s, ":") {
		parts := strings.Split(s, ":")
		h, _ := strconv.ParseInt(parts[0], 10, 64)
		m, _ := strconv.ParseInt(parts[1], 10, 64)
		sec, _ := strconv.ParseInt(parts[2], 10, 64)
		return h*3600 + m*60 + sec, nil
	}
	words := strings.Fields(s)
	var total int64
	for i := 0; i < len(words); i += 2 {
		n, err := strconv.ParseInt(words[i], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%q: %w", s, err)
		}
		unit := unitSeconds[strings.TrimSuffix(words[i+1], "s")]
		if n > math.MaxInt64/unit || total > math.MaxInt64-n*unit {
			return 0, fmt.Errorf("%q is too long", s)
		}
		total += n * unit
	}
	return total, nil
}

var (
	_ basetypes.StringTypable                    = PeriodType{}
	_ basetypes.StringValuableWithSemanticEquals = PeriodValue{}
)

// PeriodType is a string attribute holding a check-in period. The API stores
// periods in a canonical long form ("1 week" reads back as "7 days"), so
// equal durations compare equal regardless of how they're written.
type PeriodType struct {
	basetypes.StringType
}

func (t PeriodType) Equal(o attr.Type) bool {
	other, ok := o.(PeriodType)
	return ok && t.StringType.Equal(other.StringType)
}

func (t PeriodType) String() string { return "customtypes.PeriodType" }

func (t PeriodType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return PeriodValue{StringValue: in}, nil
}

func (t PeriodType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	v, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	s, ok := v.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T", v)
	}
	return PeriodValue{StringValue: s}, nil
}

func (t PeriodType) ValueType(context.Context) attr.Value { return PeriodValue{} }

// PeriodValue is a PeriodType value.
type PeriodValue struct {
	basetypes.StringValue
}

// NewPeriodValue returns a known period.
func NewPeriodValue(s string) PeriodValue {
	return PeriodValue{StringValue: basetypes.NewStringValue(s)}
}

// NewPeriodNull returns a null period.
func NewPeriodNull() PeriodValue {
	return PeriodValue{StringValue: basetypes.NewStringNull()}
}

// NewPeriodUnknown returns an unknown period.
func NewPeriodUnknown() PeriodValue {
	return PeriodValue{StringValue: basetypes.NewStringUnknown()}
}

func (v PeriodValue) Equal(o attr.Value) bool {
	other, ok := o.(PeriodValue)
	return ok && v.StringValue.Equal(other.StringValue)
}

func (v PeriodValue) Type(context.Context) attr.Type { return PeriodType{} }

// StringSemanticEquals reports whether two periods are the same length. A
// value that doesn't parse is never equal to anything; the API rejects it on
// apply with its own error.
func (v PeriodValue) StringSemanticEquals(_ context.Context, other basetypes.StringValuable) (bool, diag.Diagnostics) {
	o, ok := other.(PeriodValue)
	if !ok {
		return false, nil
	}
	a, errA := ParsePeriod(v.ValueString())
	b, errB := ParsePeriod(o.ValueString())
	return errA == nil && errB == nil && a == b, nil
}
