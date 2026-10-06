package specgen

import (
	"github.com/getkin/kin-openapi/openapi3"
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
		{Name: "gear", Kind: "object", Required: true,
			Description: "The widget's drive gear. Replaced whole on update.",
			Attributes: []Attribute{
				{Name: "kind", Kind: "string", Required: true, Enum: []string{"spur", "helical"}, Description: "Tooth profile."},
				{Name: "mesh", Kind: "object", Required: true, Attributes: []Attribute{
					{Name: "pitch", Kind: "float64", Required: true, Description: "Tooth spacing in millimetres."},
					{Name: "teeth", Kind: "int64", Required: true, Minimum: ptr(1.0), Description: "Number of teeth."},
				}},
			}},
		{Name: "id", Kind: "string", Computed: true, UseStateForUnknown: true,
			Description: "Unique identifier"},
		{Name: "name", Kind: "string", Required: true, Description: "Widget name."},
		{Name: "parts", Kind: "set", ElemKind: "string", Optional: true, Computed: true, UseStateForUnknown: true,
			MinItems: 1, MaxItems: ptr(int64(10)),
			Description: "Part numbers fitted. Left out on create, every stocked part. Removing it from config keeps the current value."},
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
			Minimum: ptr(1.0), Maximum: ptr(500.0), Description: "Size in centimetres. Removing it from config keeps the current value."},
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
		{"update-only field not omitted", func(rc *ResourceConfig) { delete(rc.Omit, "archived") }, "archived"},
		{"writeOnly non-secret not omitted", func(rc *ResourceConfig) { delete(rc.Omit, "quiet") }, "quiet"},
		{"server_filled on a computed field", func(rc *ResourceConfig) { rc.ServerFilled = append(rc.ServerFilled, "token") }, "token"},
		{"use_state_for_unknown on a defaulted field", func(rc *ResourceConfig) { rc.UseStateForUnknown = append(rc.UseStateForUnknown, "shape") }, "shape"},
		{"replace on a computed field", func(rc *ResourceConfig) { rc.Replace = append(rc.Replace, "created_at") }, "created_at"},
		{"import names a missing attribute", func(rc *ResourceConfig) { rc.Import = "<project_id>/<nope>" }, "nope"},
		{"update operation without a request body", func(rc *ResourceConfig) { rc.Update = "getWidget" }, "getWidget"},
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
		Description: "Part numbers fitted. Left out on create, every stocked part. Removing it from config keeps the current value."}
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
		{"read-only", Field{Name: "made_at", Type: "string", ReadOnly: true}, true},
		{"exclusive bound", Field{Name: "ratio", Type: "number", ExclusiveMinimum: &zero}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := Object{Required: map[string]bool{tt.field.Name: tt.required}, Fields: map[string]Field{tt.field.Name: tt.field}}
			if _, err := nestedAttributes(obj, false); err == nil || !strings.Contains(err.Error(), tt.field.Name) {
				t.Errorf("err = %v, want one naming %s", err, tt.field.Name)
			}
		})
	}
}

func TestNestedAttributesOptionalDefaultedWriteOnly(t *testing.T) {
	obj := Object{Required: map[string]bool{"url": true}, Fields: map[string]Field{
		"url":   {Name: "url", Type: "string", Nullable: true},
		"token": {Name: "token", Type: "string", Nullable: true, WriteOnly: true, Secret: true},
		"mode":  {Name: "mode", Type: "string", Nullable: true, Default: "fast"},
		"note":  {Name: "note", Type: "string", Nullable: true},
	}}
	got, err := nestedAttributes(obj, false)
	if err != nil {
		t.Fatalf("nestedAttributes: %v", err)
	}
	want := []Attribute{
		{Name: "mode", Kind: "string", Optional: true, Computed: true, Default: "fast"},
		{Name: "note", Kind: "string", Optional: true},
		{Name: "token", Kind: "string", Optional: true, Sensitive: true, NotRead: true},
		{Name: "url", Kind: "string", Required: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestNestedAttributesRejectOptionalNonNullable(t *testing.T) {
	obj := Object{Required: map[string]bool{}, Fields: map[string]Field{"note": {Name: "note", Type: "string"}}}
	if _, err := nestedAttributes(obj, false); err == nil || !strings.Contains(err.Error(), "note") {
		t.Errorf("err = %v, want one naming note", err)
	}
}

// A read-only object's keys are all Computed, whatever their nullability.
func TestNestedAttributesComputed(t *testing.T) {
	obj := Object{Required: map[string]bool{}, Fields: map[string]Field{"web": {Name: "web", Type: "string"}}}
	got, err := nestedAttributes(obj, true)
	if err != nil || len(got) != 1 || !got[0].Computed || got[0].Optional || got[0].Required {
		t.Errorf("got %+v, %v; want web Computed only", got, err)
	}
}

func TestSetKindCollections(t *testing.T) {
	a := Attribute{Name: "spot_ids", Default: []any{}}
	f := Field{Type: "array", UniqueItems: true, Items: &Field{Type: "string", Format: "uuid"}}
	if err := setKind(&a, f); err != nil || a.Kind != "set" || a.ElemFormat != "uuid" {
		t.Errorf("uuid set: %+v, %v", a, err)
	}
	b := Attribute{Name: "events"}
	g := Field{Type: "array", Items: &Field{Type: "string", Enum: []string{"spun"}}}
	if err := setKind(&b, g); err != nil || !reflect.DeepEqual(b.ElemEnum, []string{"spun"}) {
		t.Errorf("enum list: %+v, %v", b, err)
	}
	c := Attribute{Name: "filters"}
	h := Field{Type: "array", Items: &Field{Type: "object", Properties: &Object{
		Required: map[string]bool{"query": true}, Fields: map[string]Field{"query": {Name: "query", Type: "string"}}}}}
	if err := setKind(&c, h); err != nil || c.Kind != "list" || c.ElemKind != "object" || len(c.Attributes) != 1 {
		t.Errorf("object list: %+v, %v", c, err)
	}
	d := Attribute{Name: "tags", Default: []any{"x"}}
	if err := setKind(&d, Field{Type: "array", Items: &Field{Type: "string"}}); err == nil {
		t.Error("a non-empty collection default: want an error")
	}
}

func gadgetConfig(t *testing.T) ResourceConfig {
	t.Helper()
	cfg, err := LoadConfig("testdata/resources_gadget.yaml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	rc := cfg.Resources["gadget"]
	rc.Provider = cfg.Provider
	return rc
}

func TestMergeAllVariants(t *testing.T) {
	rs, err := MergeAll(loadFixture(t), "gadget", gadgetConfig(t))
	if err != nil {
		t.Fatalf("MergeAll: %v", err)
	}
	var names []string
	for _, r := range rs {
		names = append(names, r.Name)
	}
	if !reflect.DeepEqual(names, []string{"gadget_alpha", "gadget_beta", "gadget_gamma"}) {
		t.Fatalf("names = %v", names)
	}
	attr := func(r Resource, n string) Attribute {
		for _, a := range r.Attributes {
			if a.Name == n {
				return a
			}
		}
		t.Fatalf("%s has no %s", r.Name, n)
		return Attribute{}
	}
	alpha, beta, gamma := rs[0], rs[1], rs[2]
	if alpha.Variant == nil || *alpha.Variant != (VariantInfo{Field: "type", Const: "Alpha"}) {
		t.Errorf("alpha variant = %+v", alpha.Variant)
	}
	for _, a := range alpha.Attributes {
		if a.Name == "type" {
			t.Error("the discriminator is an attribute")
		}
	}
	if c := attr(alpha, "config"); !c.Required || len(c.Attributes) != 2 {
		t.Errorf("alpha config = %+v, want required with url and token", c)
	}
	if c := attr(beta, "config"); c.Required || !c.Computed {
		t.Errorf("beta config = %+v, want optional and computed (not nullable on update)", c)
	}
	if e := attr(alpha, "events"); !reflect.DeepEqual(e.ElemEnum, []string{"spun", "stopped"}) {
		t.Errorf("alpha events enum = %v", e.ElemEnum)
	}
	if e := attr(gamma, "events"); e.MaxItems == nil || *e.MaxItems != 0 {
		t.Errorf("gamma events maxItems = %v", e.MaxItems)
	}
	if a := attr(gamma, "active"); a.Default != false {
		t.Errorf("gamma active default = %v", a.Default)
	}
	if l := attr(alpha, "links"); !l.Computed || l.Kind != "object" {
		t.Errorf("links = %+v", l)
	}
}

func TestSnake(t *testing.T) {
	for in, want := range map[string]string{
		"WebHook": "web_hook", "OpsGenieV2": "ops_genie_v2", "IncidentIo": "incident_io",
		"Github": "github", "MicrosoftTeamsV2": "microsoft_teams_v2", "CampfireV3": "campfire_v3",
		"ClickUpChat": "click_up_chat", "Alpha": "alpha",
	} {
		if got := snake(in); got != want {
			t.Errorf("snake(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestMergeAllRejectsConstOutsideEnum(t *testing.T) {
	doc := loadFixture(t)
	disc := doc.Components.Schemas["GadgetCreateInput"].Value.Properties["type"].Value
	orig := disc.Enum
	disc.Enum = []any{"Alpha", "Beta"}
	defer func() { disc.Enum = orig }()
	if _, err := MergeAll(doc, "gadget", gadgetConfig(t)); err == nil || !strings.Contains(err.Error(), "Gamma") {
		t.Errorf("err = %v, want one naming Gamma", err)
	}
}

func TestMergeAllRejectsWidening(t *testing.T) {
	doc := loadFixture(t)
	gamma := doc.Components.Schemas["GadgetCreateInput"].Value.AllOf[2].Value.Then.Value
	orig := gamma.Properties["events"]
	widened := *orig.Value
	widened.Type = &openapi3.Types{"string"}
	gamma.Properties["events"] = &openapi3.SchemaRef{Value: &widened}
	defer func() { gamma.Properties["events"] = orig }()
	if _, err := MergeAll(doc, "gadget", gadgetConfig(t)); err == nil || !strings.Contains(err.Error(), "events") {
		t.Errorf("err = %v, want one naming events", err)
	}
}

// An input field without a description takes the read model's.
func TestMergeFallsBackToReadDescription(t *testing.T) {
	rs, err := MergeAll(loadFixture(t), "gadget", gadgetConfig(t))
	if err != nil {
		t.Fatalf("MergeAll: %v", err)
	}
	for _, a := range rs[0].Attributes {
		if a.Name == "active" && a.Description != "Whether the gadget runs." {
			t.Errorf("active description = %q", a.Description)
		}
	}
}

// A server-filled attribute says what removing it does, since that keeps
// the stored value rather than resetting it.
func TestServerFilledDescribesRemoval(t *testing.T) {
	res, err := Merge(loadFixture(t), "widget", fixtureConfig(t))
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	for _, a := range res.Attributes {
		if a.Name == "size" && !strings.HasSuffix(a.Description, "Removing it from config keeps the current value.") {
			t.Errorf("size description = %q", a.Description)
		}
	}
}

// A branch fact narrow doesn't apply is an error, not silently dropped.
func TestNarrowRejectsUnhandledBranchFacts(t *testing.T) {
	ten := int64(10)
	base := Object{Required: map[string]bool{}, Fields: map[string]Field{"label": {Name: "label", Type: "string"}}}
	for name, b := range map[string]Field{
		"pattern":   {Name: "label", Pattern: "^x$"},
		"maxLength": {Name: "label", MaxLength: &ten},
		"enum":      {Name: "label", Enum: []string{"a"}},
		"format":    {Name: "label", Format: "uuid"},
	} {
		branch := Object{Required: map[string]bool{}, Fields: map[string]Field{"label": b}}
		if _, err := narrow(base, branch); err == nil || !strings.Contains(err.Error(), "label") {
			t.Errorf("%s: err = %v, want one naming label", name, err)
		}
	}
}

// A write-only key is sensitive even without format: password.
func TestWriteOnlyIsSensitive(t *testing.T) {
	obj := Object{Required: map[string]bool{}, Fields: map[string]Field{
		"token": {Name: "token", Type: "string", Nullable: true, WriteOnly: true}}}
	got, err := nestedAttributes(obj, false)
	if err != nil || !got[0].Sensitive {
		t.Errorf("got %+v, %v; want token Sensitive", got, err)
	}
}

// A variant's description is its branch's, when the branch has one, so each
// type's page says what that type is; otherwise it's the shared model's.
func TestMergeAllVariantDescription(t *testing.T) {
	rs, err := MergeAll(loadFixture(t), "gadget", gadgetConfig(t))
	if err != nil {
		t.Fatalf("MergeAll: %v", err)
	}
	if rs[0].Description != "An alpha gadget, which posts to a URL." || rs[1].Description != "A gadget." {
		t.Errorf("descriptions = %q, %q", rs[0].Description, rs[1].Description)
	}
}

func attrNamed(t *testing.T, r Resource, name string) Attribute {
	t.Helper()
	for _, a := range r.Attributes {
		if a.Name == name {
			return a
		}
	}
	t.Fatalf("resource %s has no attribute %s", r.Name, name)
	return Attribute{}
}

func TestMergeStringMap(t *testing.T) {
	r, err := Merge(loadFixture(t), "sprocket", sprocketConfig(t))
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	h := attrNamed(t, r, "headers")
	if h.Kind != "map" || h.ElemKind != "string" || !h.Sensitive || !h.Optional || !h.Computed {
		t.Errorf("headers = %+v; want an optional, computed, sensitive map of strings", h)
	}
}

// A map whose values aren't strings is refused until a schema needs one.
func TestSetKindRefusesNonStringMap(t *testing.T) {
	a := Attribute{Name: "counts"}
	err := setKind(&a, Field{Type: "object", Values: &Field{Type: "integer"}})
	if err == nil || !strings.Contains(err.Error(), "values are strings") {
		t.Errorf("err = %v, want a refusal naming string values", err)
	}
}

func TestMergeIntegerEnum(t *testing.T) {
	r, err := Merge(loadFixture(t), "sprocket", sprocketConfig(t))
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	s := attrNamed(t, r, "speed")
	if s.Kind != "int64" || !reflect.DeepEqual(s.Enum, []string{"1", "2", "5", "15"}) || s.Default != float64(5) {
		t.Errorf("speed = %+v; want an int64 enum defaulting to 5", s)
	}
}

func TestMergeEmptyObjectDefault(t *testing.T) {
	r, err := Merge(loadFixture(t), "sprocket", sprocketConfig(t))
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	tr := attrNamed(t, r, "trim")
	if tr.Kind != "object" || !tr.Optional || !tr.Computed || tr.Default == nil {
		t.Errorf("trim = %+v; want an optional, computed object with a default", tr)
	}
}

func TestEmptyObjectDefaultRefusals(t *testing.T) {
	for name, a := range map[string]Attribute{
		"non-empty":      {Name: "o", Kind: "object", Default: map[string]any{"color": "red"}},
		"required child": {Name: "o", Kind: "object", Default: map[string]any{}, Attributes: []Attribute{{Name: "c", Kind: "string", Required: true}}},
		"object child":   {Name: "o", Kind: "object", Default: map[string]any{}, Attributes: []Attribute{{Name: "c", Kind: "object", Optional: true}}},
	} {
		if err := emptyObjectDefault(&a); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
