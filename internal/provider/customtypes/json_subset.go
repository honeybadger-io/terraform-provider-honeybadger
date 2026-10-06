package customtypes

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var (
	_ basetypes.StringTypable                    = JSONSubsetType{}
	_ basetypes.StringValuableWithSemanticEquals = JSONSubsetValue{}
	_ validator.String                           = JSONValidator{}
)

// JSONSubsetType is a string attribute holding a JSON document the API may
// add to: keys it fills in and empty values it drops. What the user wrote
// compares equal to what the API returns when it's a subset of it (phase 6
// spec, 3.1), so state keeps the user's JSON.
type JSONSubsetType struct {
	basetypes.StringType
}

func (t JSONSubsetType) Equal(o attr.Type) bool {
	other, ok := o.(JSONSubsetType)
	return ok && t.StringType.Equal(other.StringType)
}

func (t JSONSubsetType) String() string { return "customtypes.JSONSubsetType" }

func (t JSONSubsetType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return JSONSubsetValue{StringValue: in}, nil
}

func (t JSONSubsetType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	v, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	s, ok := v.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T", v)
	}
	return JSONSubsetValue{StringValue: s}, nil
}

func (t JSONSubsetType) ValueType(context.Context) attr.Value { return JSONSubsetValue{} }

// JSONSubsetValue is a JSONSubsetType value.
type JSONSubsetValue struct {
	basetypes.StringValue
}

// NewJSONSubsetValue returns a known JSON value.
func NewJSONSubsetValue(s string) JSONSubsetValue {
	return JSONSubsetValue{StringValue: basetypes.NewStringValue(s)}
}

// NewJSONSubsetNull returns a null JSON value.
func NewJSONSubsetNull() JSONSubsetValue {
	return JSONSubsetValue{StringValue: basetypes.NewStringNull()}
}

func (v JSONSubsetValue) Equal(o attr.Value) bool {
	other, ok := o.(JSONSubsetValue)
	return ok && v.StringValue.Equal(other.StringValue)
}

func (v JSONSubsetValue) Type(context.Context) attr.Type { return JSONSubsetType{} }

// StringSemanticEquals is called on the value the provider just read, with
// the planned or prior value (what the user wrote) as other. They're equal
// when other is a subset of v. Invalid JSON is never equal.
func (v JSONSubsetValue) StringSemanticEquals(_ context.Context, other basetypes.StringValuable) (bool, diag.Diagnostics) {
	o, ok := other.(JSONSubsetValue)
	if !ok {
		return false, nil
	}
	got, errGot := parseJSON(v.ValueString())
	want, errWant := parseJSON(o.ValueString())
	if errGot != nil || errWant != nil {
		return false, nil
	}
	return Subset(want, got), nil
}

// parseJSON keeps numbers exact (json.Number), so large integers that a
// float64 would round together still compare unequal.
func parseJSON(s string) (any, error) {
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// Subset reports whether want is contained in got: a key whose value is
// "not set" matches anything (a whole document or array element is a value,
// so [] doesn't match a non-empty list); every key want sets matches got's; arrays holding any
// object or array match position by position; arrays of scalars match as
// multisets; scalars match by value (numbers numerically).
func Subset(want, got any) bool {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for k, wv := range w {
			if notSet(wv) {
				continue
			}
			if !Subset(wv, g[k]) {
				return false
			}
		}
		return true
	case []any:
		g, ok := got.([]any)
		if !ok {
			return false
		}
		if scalars(w) && scalars(g) {
			return sameMultiset(w, g)
		}
		if len(w) != len(g) {
			return false
		}
		for i := range w {
			if !Subset(w[i], g[i]) {
				return false
			}
		}
		return true
	case json.Number:
		g, ok := got.(json.Number)
		return ok && scalarKey(w) == scalarKey(g)
	}
	return want == got
}

// scalarKey identifies a scalar for comparison: numbers by exact value
// (3 and 3.0 alike), everything else by type and value.
func scalarKey(v any) string {
	if n, ok := v.(json.Number); ok {
		if r, ok := new(big.Rat).SetString(n.String()); ok {
			return "number:" + r.RatString()
		}
	}
	return fmt.Sprintf("%T:%v", v, v)
}

// notSet is what the API drops on read: null, "", [], {}, and an object
// that is empty once those are removed.
func notSet(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case map[string]any:
		for _, e := range x {
			if !notSet(e) {
				return false
			}
		}
		return true
	}
	return false
}

func scalars(a []any) bool {
	for _, e := range a {
		switch e.(type) {
		case map[string]any, []any:
			return false
		}
	}
	return true
}

func sameMultiset(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	counts := map[string]int{}
	for _, e := range a {
		counts[scalarKey(e)]++
	}
	for _, e := range b {
		k := scalarKey(e)
		if counts[k] == 0 {
			return false
		}
		counts[k]--
	}
	return true
}

// JSONValidator checks, at plan time, that a JSON attribute holds its
// schema's top-level type ("array" or "object") and, for an array, that each
// element is an object setting every Required key.
type JSONValidator struct {
	Type     string
	Required []string
}

func (j JSONValidator) Description(context.Context) string {
	return "must be valid JSON of the expected shape"
}

func (j JSONValidator) MarkdownDescription(ctx context.Context) string { return j.Description(ctx) }

func (j JSONValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	var v any
	if err := json.Unmarshal([]byte(req.ConfigValue.ValueString()), &v); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid JSON", fmt.Sprintf("%s isn't valid JSON: %v", req.Path, err))
		return
	}
	switch j.Type {
	case "array":
		arr, ok := v.([]any)
		if !ok {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid JSON", fmt.Sprintf("%s must be a JSON array", req.Path))
			return
		}
		for i, e := range arr {
			obj, ok := e.(map[string]any)
			if !ok {
				if len(j.Required) > 0 {
					resp.Diagnostics.AddAttributeError(req.Path, "Invalid JSON", fmt.Sprintf("%s[%d] must be a JSON object", req.Path, i))
				}
				continue
			}
			for _, k := range j.Required {
				if notSet(obj[k]) {
					resp.Diagnostics.AddAttributeError(req.Path, "Missing required key", fmt.Sprintf("%s[%d]: %q is required", req.Path, i, k))
				}
			}
		}
	case "object":
		if _, ok := v.(map[string]any); !ok {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid JSON", fmt.Sprintf("%s must be a JSON object", req.Path))
		}
	}
}
