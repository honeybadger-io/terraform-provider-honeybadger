package specgen

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixtureConfig(t *testing.T) ResourceConfig {
	t.Helper()
	cfg, err := LoadConfig("testdata/resources.yaml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return cfg.Resources["widget"]
}

func TestMergeFollowsTheMergeRule(t *testing.T) {
	res, err := Merge(loadFixture(t), "widget", fixtureConfig(t))
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	want := []Attribute{
		{Name: "badges", Kind: "set", ElemKind: "string", Optional: true, Description: "Badges awarded to the widget."},
		{Name: "color", Kind: "string", Optional: true, Pattern: "^#[0-9a-f]{6}$",
			Description: "The widget's color, as a hex code."},
		{Name: "created_at", Kind: "string", Computed: true, UseStateForUnknown: true,
			Description: "When it was made"},
		{Name: "id", Kind: "string", Computed: true, UseStateForUnknown: true,
			Description: "Unique identifier"},
		{Name: "name", Kind: "string", Required: true, Description: "Widget name."},
		{Name: "period", Kind: "string", Optional: true, CustomType: "period",
			Description: "How often it spins, such as `5 minutes`."},
		{Name: "project_id", Kind: "string", Required: true, RequiresReplace: true,
			Description: "The project's ID."},
		{Name: "secret", Kind: "string", Optional: true, Sensitive: true, NotRead: true,
			Description: "Webhook signing secret."},
		{Name: "serial", Kind: "string", Optional: true, RequiresReplace: true, MaxLength: ptr(int64(32)),
			Description: "Factory serial number, set once."},
		{Name: "shape", Kind: "string", Optional: true, Computed: true, Default: "round",
			Enum: []string{"round", "square"}, Description: "How the widget looks."},
		{Name: "size", Kind: "int64", Optional: true, Computed: true, UseStateForUnknown: true,
			Minimum: ptr(1.0), Maximum: ptr(500.0), Description: "Size in centimetres."},
		{Name: "tags", Kind: "set", ElemKind: "string", Optional: true, Description: "Labels on the widget."},
		{Name: "token", Kind: "string", Computed: true, Sensitive: true,
			Description: "Credential for the widget"},
	}
	if res.Import != "<project_id>/<id>" {
		t.Errorf("import = %q", res.Import)
	}
	if !reflect.DeepEqual(res.Parents, []string{"project_id"}) || res.IDParam != "widget_id" {
		t.Errorf("parents = %v, id param = %q, want [project_id] and widget_id", res.Parents, res.IDParam)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "token") {
		t.Errorf("warnings = %v, want one about the redundant sensitive override on token", res.Warnings)
	}
	if !reflect.DeepEqual(res.Attributes, want) {
		for i := range want {
			if i >= len(res.Attributes) || !reflect.DeepEqual(res.Attributes[i], want[i]) {
				var got any = "missing"
				if i < len(res.Attributes) {
					got = res.Attributes[i]
				}
				t.Errorf("attribute %d:\n got  %+v\n want %+v", i, got, want[i])
			}
		}
		if len(res.Attributes) != len(want) {
			t.Errorf("got %d attributes, want %d", len(res.Attributes), len(want))
		}
	}
}

func TestMergeErrors(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(rc *ResourceConfig)
		wantErr string
	}{
		{"input-only field not omitted", func(rc *ResourceConfig) { delete(rc.Omit, "note") }, "note"},
		{"array item enum not omitted", func(rc *ResourceConfig) { delete(rc.Omit, "events") }, "events"},
		{"update-only field not omitted", func(rc *ResourceConfig) { delete(rc.Omit, "archived") }, "archived"},
		{"writeOnly non-secret not omitted", func(rc *ResourceConfig) { delete(rc.Omit, "quiet") }, "quiet"},
		{"server_filled on a computed field", func(rc *ResourceConfig) { rc.ServerFilled = append(rc.ServerFilled, "token") }, "token"},
		{"use_state_for_unknown on a defaulted field", func(rc *ResourceConfig) { rc.UseStateForUnknown = append(rc.UseStateForUnknown, "shape") }, "shape"},
		{"replace on a computed field", func(rc *ResourceConfig) { rc.Replace = append(rc.Replace, "created_at") }, "created_at"},
		{"import names a missing attribute", func(rc *ResourceConfig) { rc.Import = "<project_id>/<nope>" }, "nope"},
		{"update operation without a request body", func(rc *ResourceConfig) { rc.Update = "getWidget" }, "getWidget"},
		{"nested object not omitted", func(rc *ResourceConfig) { delete(rc.Omit, "links") }, "links"},
		{"server_filled names a nested attribute", func(rc *ResourceConfig) { rc.ServerFilled = append(rc.ServerFilled, "teeth") }, "teeth"},
		{"override names a missing field", func(rc *ResourceConfig) { rc.Sensitive = append(rc.Sensitive, "nope") }, "nope"},
		{"unknown custom type", func(rc *ResourceConfig) { rc.CustomTypes = map[string]string{"period": "nope"} }, "nope"},
		{"unknown operation", func(rc *ResourceConfig) { rc.Read = "getGizmo" }, "getGizmo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := fixtureConfig(t)
			omit := map[string]string{}
			for k, v := range rc.Omit {
				omit[k] = v
			}
			rc.Omit = omit
			tt.edit(&rc)
			_, err := Merge(loadFixture(t), "widget", rc)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "widget") {
				t.Errorf("err = %v, want one naming widget and %q", err, tt.wantErr)
			}
		})
	}
}

func TestStripCallerWording(t *testing.T) {
	tests := map[string]string{
		"Widget name. Send null to clear.":                                                        "Widget name.",
		"Zero is `00:00:00`. Send null to reset to zero.":                                         "Zero is `00:00:00`.",
		"Send null to clear; an unnamed check-in shows its ID.":                                   "An unnamed check-in shows its ID.",
		"No caller wording here.":                                                                 "No caller wording here.",
		"Context key, when not in `context.user_email` or `context.user_id`. Send null to clear.": "Context key, when not in `context.user_email` or `context.user_id`.",
	}
	for in, want := range tests {
		if got := stripCallerWording(in); got != want {
			t.Errorf("stripCallerWording(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadConfigReadsCRUDKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.yaml")
	src := "resources:\n  w:\n    service: Widgets\n    api:\n      model: Widget\n      create_params: WidgetCreateParams\n      update_params: WidgetUpdateParams\n    find_hint: the widgets\n    config_validators: widgetValidators\n    crud: manual\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	w := cfg.Resources["w"]
	if w.Service != "Widgets" || w.API.Model != "Widget" || w.API.CreateParams != "WidgetCreateParams" ||
		w.API.UpdateParams != "WidgetUpdateParams" || w.FindHint != "the widgets" ||
		w.ConfigValidators != "widgetValidators" || w.CRUD != "manual" {
		t.Errorf("config = %+v", w)
	}
}

func TestMergeRejectsUnknownCRUDMode(t *testing.T) {
	rc := fixtureConfig(t)
	rc.CRUD = "automatic"
	_, err := Merge(loadFixture(t), "widget", rc)
	if err == nil || !strings.Contains(err.Error(), "automatic") {
		t.Errorf("err = %v, want one naming the crud value", err)
	}
}

// mergeNested merges the widget with gear and parts, which the fixture
// config omits until objects render.
func mergeNested(t *testing.T) Resource {
	t.Helper()
	rc := fixtureConfig(t)
	omit := map[string]string{}
	for k, v := range rc.Omit {
		if k != "gear" && k != "parts" {
			omit[k] = v
		}
	}
	rc.Omit = omit
	rc.ServerFilled = append(rc.ServerFilled, "parts")
	res, err := Merge(loadFixture(t), "widget", rc)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	return res
}

func TestMergeNestedAndCollections(t *testing.T) {
	res := mergeNested(t)
	got := map[string]Attribute{}
	for _, a := range res.Attributes {
		got[a.Name] = a
	}
	wantGear := Attribute{Name: "gear", Kind: "object", Required: true,
		Description: "The widget's drive gear. Replaced whole on update.",
		Attributes: []Attribute{
			{Name: "kind", Kind: "string", Required: true, Enum: []string{"spur", "helical"}, Description: "Tooth profile."},
			{Name: "mesh", Kind: "object", Required: true, Attributes: []Attribute{
				{Name: "pitch", Kind: "float64", Required: true, Description: "Tooth spacing in millimetres."},
				{Name: "teeth", Kind: "int64", Required: true, Minimum: ptr(1.0), Description: "Number of teeth."},
			}},
		}}
	wantParts := Attribute{Name: "parts", Kind: "set", ElemKind: "string", Optional: true, Computed: true,
		UseStateForUnknown: true, MinItems: 1, MaxItems: ptr(int64(10)),
		Description: "Part numbers fitted. Left out on create, every stocked part."}
	if !reflect.DeepEqual(got["gear"], wantGear) {
		t.Errorf("gear:\n got  %+v\n want %+v", got["gear"], wantGear)
	}
	if !reflect.DeepEqual(got["parts"], wantParts) {
		t.Errorf("parts:\n got  %+v\n want %+v", got["parts"], wantParts)
	}
	if res.Description != "A widget in a project." {
		t.Errorf("Description = %q", res.Description)
	}
}

// Inside an object, only required attributes are supported today. The rest
// need their own rules, because the object is replaced whole, and wait until
// the bundle needs them (phase 4a spec, section 3.1).
func TestNestedAttributesRejectUnsupported(t *testing.T) {
	zero := 0.0
	tests := []struct {
		name     string
		field    Field
		required bool
	}{
		{"optional", Field{Name: "note", Type: "string"}, false},
		{"defaulted", Field{Name: "mode", Type: "string", Default: "static"}, true},
		{"read-only", Field{Name: "made_at", Type: "string", ReadOnly: true}, true},
		{"write-only", Field{Name: "key", Type: "string", WriteOnly: true}, true},
		{"exclusive bound", Field{Name: "ratio", Type: "number", ExclusiveMinimum: &zero}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := Object{Required: map[string]bool{tt.field.Name: tt.required}, Fields: map[string]Field{tt.field.Name: tt.field}}
			if _, err := nestedAttributes(obj); err == nil || !strings.Contains(err.Error(), tt.field.Name) {
				t.Errorf("err = %v, want one naming %s", err, tt.field.Name)
			}
		})
	}
}

// Arrays of objects wait for phase 4b (integration filters).
func TestSetKindRejectsArrayOfObjects(t *testing.T) {
	a := Attribute{Name: "filters"}
	f := Field{Name: "filters", Type: "array", Items: &Field{Type: "object", Properties: &Object{}}}
	if err := setKind(&a, f); err == nil || !strings.Contains(err.Error(), "arrays of object") {
		t.Errorf("err = %v, want arrays of object aren't supported", err)
	}
}
