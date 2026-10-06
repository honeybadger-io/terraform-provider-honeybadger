package specgen

import (
	"reflect"
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
		Description: "How the widget looks. Send null to reset to `round`."}
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
				Description: "Size in centimetres. Send null to clear."}},
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

func ptr[T any](v T) *T { return &v }
