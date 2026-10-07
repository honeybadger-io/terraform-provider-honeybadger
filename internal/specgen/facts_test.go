package specgen

import (
	"reflect"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func loadFixture(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := Load("testdata/openapi.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return doc
}

func schemaFacts(t *testing.T, doc *openapi3.T, name string) Object {
	t.Helper()
	obj, err := ObjectFacts(name, doc.Components.Schemas[name])
	if err != nil {
		t.Fatalf("ObjectFacts(%s): %v", name, err)
	}
	return obj
}

// Review Focus 1: the property's own description wins over the $ref's, while
// the enum and default still come from the $ref.
func TestFieldFactsThroughAllOfRef(t *testing.T) {
	shape := schemaFacts(t, loadFixture(t), "WidgetInput").Fields["shape"]
	want := Field{Name: "shape", Type: "string", Enum: []string{"round", "square"}, Default: "round",
		Description: "How the widget looks. Null resets it to `round`."}
	if !reflect.DeepEqual(shape, want) {
		t.Errorf("shape = %+v, want %+v", shape, want)
	}
}

// Review Focus 2: a oneOf with a null branch is a nullable value of the other
// branch's type, keeping that branch's pattern.
func TestFieldFactsNullableUnion(t *testing.T) {
	color := schemaFacts(t, loadFixture(t), "WidgetInput").Fields["color"]
	if color.Type != "string" || !color.Nullable || color.Pattern != "^#[0-9a-f]{6}$" {
		t.Errorf("color = %+v, want a nullable string with its pattern", color)
	}
}

func TestFieldFacts(t *testing.T) {
	doc := loadFixture(t)
	widget := schemaFacts(t, doc, "Widget")
	input := schemaFacts(t, doc, "WidgetInput")

	tests := []struct {
		name string
		got  Field
		want Field
	}{
		{"type array nullable, with bounds", input.Fields["size"],
			Field{Name: "size", Type: "integer", Nullable: true, Minimum: ptr(1.0), Maximum: ptr(500.0),
				Description: "Size in centimetres. Null clears it."}},
		{"readOnly with format", widget.Fields["created_at"],
			Field{Name: "created_at", Type: "string", Format: "date-time", ReadOnly: true, Description: "When it was made"}},
		{"writeOnly secret", input.Fields["secret"],
			Field{Name: "secret", Type: "string", Format: "password", Nullable: true, WriteOnly: true, Secret: true,
				Description: "Webhook signing secret."}},
		{"unique array", input.Fields["tags"],
			Field{Name: "tags", Type: "array", UniqueItems: true, Description: "Labels on the widget.",
				Items: &Field{Name: "tags", Type: "string"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Errorf("got %+v, want %+v", tt.got, tt.want)
			}
		})
	}
}

func TestObjectFactsFlattensAllOf(t *testing.T) {
	create := schemaFacts(t, loadFixture(t), "WidgetCreateInput")
	if !create.Required["name"] {
		t.Error("name isn't required in WidgetCreateInput")
	}
	for _, f := range []string{"name", "shape", "size", "color", "tags", "period", "secret", "serial", "note", "quiet"} {
		if _, ok := create.Fields[f]; !ok {
			t.Errorf("WidgetCreateInput is missing %s", f)
		}
	}
}

// The real bundle reads as the spec says (sections 3.3 and 5.2).
func TestRealBundleFacts(t *testing.T) {
	doc, err := Load("../../openapi/bundled.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	input := schemaFacts(t, doc, "CheckInInput")

	st := input.Fields["schedule_type"]
	if st.Type != "string" || st.Nullable || st.Default != "simple" || !reflect.DeepEqual(st.Enum, []string{"simple", "cron"}) {
		t.Errorf("schedule_type = %+v, want a non-nullable string enum [simple cron] defaulting to simple", st)
	}
	if !hasPrefix(st.Description, "Switching to `simple`") {
		t.Errorf("schedule_type description = %q, want the property's own", st.Description)
	}
	if gp := input.Fields["grace_period"]; gp.Default != "00:00:00" || !gp.Nullable || gp.Pattern == "" {
		t.Errorf("grace_period = %+v, want nullable with default 00:00:00 and a pattern", gp)
	}
	if !schemaFacts(t, doc, "CheckIn").Fields["links"].ReadOnly {
		t.Error("CheckIn.links isn't readOnly")
	}
	if !schemaFacts(t, doc, "ProjectCreateInput").Required["name"] {
		t.Error("ProjectCreateInput doesn't require name")
	}
}

func hasPrefix(s, prefix string) bool { return len(s) >= len(prefix) && s[:len(prefix)] == prefix }

// An object property keeps its own properties, recursively, with their
// required lists.
func TestFieldFactsNestedObject(t *testing.T) {
	gear := schemaFacts(t, loadFixture(t), "WidgetInput").Fields["gear"]
	if gear.Type != "object" || gear.Properties == nil {
		t.Fatalf("gear = %+v, want an object with properties", gear)
	}
	if !gear.Properties.Required["kind"] || !gear.Properties.Required["mesh"] {
		t.Errorf("gear required = %v, want kind and mesh", gear.Properties.Required)
	}
	if gear.Description != "The widget's drive gear. Replaced whole on update." {
		t.Errorf("gear description = %q", gear.Description)
	}
	mesh := gear.Properties.Fields["mesh"]
	if mesh.Properties == nil || !mesh.Properties.Required["teeth"] || !mesh.Properties.Required["pitch"] {
		t.Fatalf("mesh = %+v, want an object requiring teeth and pitch", mesh)
	}
	if pitch := mesh.Properties.Fields["pitch"]; pitch.Type != "number" {
		t.Errorf("pitch = %+v, want a number", pitch)
	}
}

// A nullable anyOf of an object keeps the object's properties.
func TestFieldFactsNullableObject(t *testing.T) {
	gear := schemaFacts(t, loadFixture(t), "Widget").Fields["gear"]
	if !gear.Nullable || gear.Type != "object" || gear.Properties == nil || gear.Description != "The widget's drive gear." {
		t.Errorf("gear = %+v, want a nullable object with properties and its own description", gear)
	}
}

func TestFieldFactsItemCounts(t *testing.T) {
	parts := schemaFacts(t, loadFixture(t), "WidgetInput").Fields["parts"]
	if parts.MinItems != 1 || parts.MaxItems == nil || *parts.MaxItems != 10 || parts.Items == nil || parts.Items.Type != "string" {
		t.Errorf("parts = %+v, want 1 to 10 strings", parts)
	}
}

// Exclusive bounds are read, in both OpenAPI forms, so Merge can refuse them.
func TestFieldFactsExclusiveBounds(t *testing.T) {
	zero, yes := 0.0, true
	for name, s := range map[string]*openapi3.Schema{
		"3.1 number":  {Type: &openapi3.Types{"number"}, ExclusiveMin: openapi3.ExclusiveBound{Value: &zero}},
		"3.0 boolean": {Type: &openapi3.Types{"number"}, Min: &zero, ExclusiveMin: openapi3.ExclusiveBound{Bool: &yes}},
	} {
		f, err := FieldFacts("ratio", &openapi3.SchemaRef{Value: s})
		if err != nil || f.ExclusiveMinimum == nil || *f.ExclusiveMinimum != 0 || f.Minimum != nil {
			t.Errorf("%s: f = %+v, err = %v; want exclusiveMinimum 0 and no minimum", name, f, err)
		}
	}
}

func TestObjectFactsDescription(t *testing.T) {
	if d := schemaFacts(t, loadFixture(t), "Widget").Description; d != "A widget in a project." {
		t.Errorf("Widget description = %q", d)
	}
}

func TestVariants(t *testing.T) {
	doc := loadFixture(t)
	vs, err := Variants(doc.Components.Schemas["GadgetCreateInput"], "type")
	if err != nil {
		t.Fatalf("Variants: %v", err)
	}
	var consts []string
	for _, v := range vs {
		consts = append(consts, v.Const)
	}
	if !reflect.DeepEqual(consts, []string{"Alpha", "Beta", "Gamma"}) {
		t.Fatalf("consts = %v", consts)
	}
	alpha := vs[0].Then
	cfg := alpha.Fields["config"]
	if !alpha.Required["config"] || cfg.Properties == nil || !cfg.Properties.Required["url"] {
		t.Errorf("alpha then = %+v, want required config with required url", alpha)
	}
	if !cfg.Properties.Fields["token"].WriteOnly {
		t.Error("alpha token isn't writeOnly")
	}
	if ev := alpha.Fields["events"]; ev.Items == nil || len(ev.Items.Enum) != 2 {
		t.Errorf("alpha events = %+v, want an item enum of 2", ev)
	}
	if d := alpha.Fields["active"].Default; d != true {
		t.Errorf("alpha active default = %v", d)
	}
	if mx := vs[2].Then.Fields["events"].MaxItems; mx == nil || *mx != 0 {
		t.Errorf("gamma events maxItems = %v", mx)
	}
}

// Malformed unions are generation errors (phase 4b spec, 3.1).
func TestVariantsRejectMalformedBranches(t *testing.T) {
	str := &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}}
	branch := func(c any) *openapi3.SchemaRef {
		return &openapi3.SchemaRef{Value: &openapi3.Schema{
			If: &openapi3.SchemaRef{Value: &openapi3.Schema{Properties: openapi3.Schemas{
				"type": {Value: &openapi3.Schema{Const: c}}}}},
			Then: &openapi3.SchemaRef{Value: &openapi3.Schema{}},
		}}
	}
	tests := map[string]openapi3.SchemaRefs{
		"if without then": {{Value: &openapi3.Schema{If: branch("A").Value.If}}},
		"if without const": {{Value: &openapi3.Schema{If: &openapi3.SchemaRef{Value: &openapi3.Schema{
			Properties: openapi3.Schemas{"type": str}}}, Then: &openapi3.SchemaRef{Value: &openapi3.Schema{}}}}},
		"duplicate const": {branch("A"), branch("A")},
	}
	for name, allOf := range tests {
		_, err := Variants(&openapi3.SchemaRef{Value: &openapi3.Schema{AllOf: allOf}}, "type")
		if err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// This one pins existing behaviour (format and defaults were already read);
// it may pass before the implementation.
func TestFieldFactsUUIDItemsAndCollectionDefault(t *testing.T) {
	spots := schemaFacts(t, loadFixture(t), "GadgetCreateInput").Fields["spot_ids"]
	if spots.Items == nil || spots.Items.Format != "uuid" {
		t.Errorf("spot_ids items = %+v, want format uuid", spots.Items)
	}
	if d, ok := spots.Default.([]any); !ok || len(d) != 0 {
		t.Errorf("spot_ids default = %#v, want []", spots.Default)
	}
}

func TestFieldFactsStringMap(t *testing.T) {
	h := schemaFacts(t, loadFixture(t), "SprocketInput").Fields["headers"]
	if h.Type != "object" || !h.Nullable || h.Values == nil || h.Values.Type != "string" || !h.Values.Secret {
		t.Errorf("headers = %+v, values %+v; want a nullable object of secret strings", h, h.Values)
	}
	if d, ok := h.Default.(map[string]any); !ok || len(d) != 0 {
		t.Errorf("headers default = %#v, want {}", h.Default)
	}
}

// Conditional (if/then) allOf parts are recorded, not refused: a json_subset
// field holds them as JSON, and setKind refuses to type them (phase 6).
func TestFieldFactsConditionalAllOf(t *testing.T) {
	s := &openapi3.Schema{Type: &openapi3.Types{"object"}, AllOf: openapi3.SchemaRefs{
		{Value: &openapi3.Schema{If: &openapi3.SchemaRef{Value: &openapi3.Schema{}}, Then: &openapi3.SchemaRef{Value: &openapi3.Schema{}}}},
		{Value: &openapi3.Schema{If: &openapi3.SchemaRef{Value: &openapi3.Schema{}}, Then: &openapi3.SchemaRef{Value: &openapi3.Schema{}}}},
	}, Properties: openapi3.Schemas{"type": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}}}}
	f, err := FieldFacts("widget", &openapi3.SchemaRef{Value: s})
	if err != nil || !f.Conditional || f.Type != "object" {
		t.Fatalf("FieldFacts = %+v, %v; want a conditional object", f, err)
	}
	a := Attribute{Name: "widget"}
	if err := setKind(&a, f); err == nil || !strings.Contains(err.Error(), "json_subset") {
		t.Errorf("setKind err = %v, want a refusal pointing at json_subset", err)
	}
	arr := Field{Type: "array", Items: &f}
	if err := setKind(&Attribute{Name: "widgets"}, arr); err == nil || !strings.Contains(err.Error(), "json_subset") {
		t.Errorf("setKind(array of conditional) err = %v", err)
	}
}

// allOf [$ref, {required: [...]}] adds required keys to the referenced
// object (honeybadger b1c36c0dc9: integration branches' config). A part that
// does anything besides `required` is still refused.
func TestFieldFactsAllOfRequiredOnly(t *testing.T) {
	obj := &openapi3.Schema{Type: &openapi3.Types{"object"}, Properties: openapi3.Schemas{
		"url":   {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		"label": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
	}}
	s := &openapi3.Schema{AllOf: openapi3.SchemaRefs{
		{Ref: "#/components/schemas/Config", Value: obj},
		{Value: &openapi3.Schema{Required: []string{"url"}}},
	}}
	f, err := FieldFacts("config", &openapi3.SchemaRef{Value: s})
	if err != nil {
		t.Fatalf("FieldFacts: %v", err)
	}
	if f.Properties == nil || !f.Properties.Required["url"] || f.Properties.Required["label"] {
		t.Errorf("config = %+v; want url required, label not", f.Properties)
	}
	if obj.Required != nil {
		t.Error("the referenced schema was changed; required belongs to this use only")
	}
	bad := &openapi3.Schema{AllOf: openapi3.SchemaRefs{
		{Value: obj},
		{Value: &openapi3.Schema{Required: []string{"url"}, MinProps: 1}},
	}}
	if _, err := FieldFacts("config", &openapi3.SchemaRef{Value: bad}); err == nil {
		t.Error("an allOf part that does more than require keys should be refused")
	}
}

func ptr[T any](v T) *T { return &v }
