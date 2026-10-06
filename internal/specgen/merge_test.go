package specgen

import (
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
		{Name: "secret", Kind: "string", Optional: true, Sensitive: true,
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
