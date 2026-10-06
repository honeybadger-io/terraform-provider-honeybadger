package specgen

import (
	"reflect"
	"strings"
	"testing"
)

func crudConfig(t *testing.T) ResourceConfig {
	t.Helper()
	cfg, err := LoadConfig("testdata/resources_crud.yaml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return cfg.Resources["widget"]
}

func mapWidget(t *testing.T, edit func(*ResourceConfig)) (*Mapping, error) {
	t.Helper()
	rc := crudConfig(t)
	omit := map[string]string{}
	for k, v := range rc.Omit {
		omit[k] = v
	}
	rc.Omit = omit
	if edit != nil {
		edit(&rc)
	}
	res, err := Merge(loadFixture(t), "widget", rc)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	return Map(loadFakeAPI(t), res, rc)
}

func TestMapBuildsStatementsByShape(t *testing.T) {
	m, err := mapWidget(t, nil)
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
	wantCreate := []string{
		"p.Color = createString(m.Color)",
		`p.Gear = orZero(widgetGearToAPI(m.Gear, path.Root("gear"), diags))`,
		"p.Name = m.Name.ValueString()",
		"setSlicePtr(&p.Parts, m.Parts, fromString)",
		"p.Period = createString(m.Period.StringValue)",
		"p.Secret = createString(m.Secret)",
		"p.Serial = createStringPtr(m.Serial)",
		"setEnumPtr(&p.Shape, m.Shape)",
		"p.Size = createInt(m.Size)",
	}
	wantUpdate := []string{
		"p.Color = updateString(m.Color, state.Color)",
		`p.Gear = changedPtr(m.Gear, state.Gear, widgetGearToAPI(m.Gear, path.Root("gear"), diags))`,
		"p.Name = updateStringPtr(m.Name, state.Name)",
		"updateNullableSlice(&p.Parts, m.Parts, state.Parts, fromString)",
		"p.Period = updateString(m.Period.StringValue, state.Period.StringValue)",
		"p.Secret = updateString(m.Secret, state.Secret)",
		"updateEnumPtr(&p.Shape, m.Shape, state.Shape)",
		"p.Size = updateInt(m.Size, state.Size)",
	}
	wantRead := []string{
		"m.Color = stringValue(c.Color)",
		"m.CreatedAt = timeValue(c.CreatedAt)",
		"m.Gear = widgetGearFromAPI(nullablePtr(c.Gear))",
		"m.ID = types.StringValue(c.Id)",
		"m.Name = types.StringValue(c.Name)",
		"m.Parts = setOf(nullableSlice(c.Parts), types.StringType, stringElem)",
		"m.Period = periodValue(c.Period)",
		"m.ProjectID = types.StringValue(c.ProjectId)",
		"m.Serial = stringPtrValue(c.Serial)",
		"m.Shape = types.StringValue(string(c.Shape))",
		"m.Size = int64Value(c.Size)",
		"m.Token = types.StringValue(c.Token)",
	}
	for name, pair := range map[string][2][]string{
		"create": {m.CreateBody, wantCreate}, "update": {m.UpdateBody, wantUpdate}, "read": {m.ReadBody, wantRead},
	} {
		if !reflect.DeepEqual(pair[0], pair[1]) {
			t.Errorf("%s body:\n got  %q\n want %q", name, pair[0], pair[1])
		}
	}
	if !m.Wrapper || m.IDField != "Id" || m.NameAttr != "Name" || !m.Period ||
		!reflect.DeepEqual(m.Parents, []string{"project_id"}) {
		t.Errorf("mapping = %+v", m)
	}
}

// Review Focus 2 and the other generation errors.
func TestMapErrors(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(*ResourceConfig)
		wantErr string
	}{
		{"array field mapped", func(rc *ResourceConfig) { delete(rc.Omit, "tags") }, "tags"},
		{"model missing a field", func(rc *ResourceConfig) { rc.API.Model = "WidgetPartial" }, "color"},
		{"non-required value on create", func(rc *ResourceConfig) { rc.API.CreateParams = "WidgetBadCreateParams" }, "serial"},
		{"path parameters out of order", func(rc *ResourceConfig) { rc.Service = "WidgetsSwapped" }, "project_id"},
		{"optional enum as a plain value", func(rc *ResourceConfig) { rc.API.CreateParams = "WidgetEnumCreateParams" }, "shape"},
		{"missing method", func(rc *ResourceConfig) { rc.Service = "WidgetsNoDelete" }, "Delete"},
		{"no import format", func(rc *ResourceConfig) { rc.Import = "" }, "import"},
		{"removable slice as a pointer on update", func(rc *ResourceConfig) {
			rc.ServerFilled = []string{"size"}
			rc.API.UpdateParams = "WidgetSlicePointerUpdateParams"
		}, "parts"},
		{"nested object as a map", func(rc *ResourceConfig) { rc.API.UpdateParams = "WidgetMapGearUpdateParams" }, "gear"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mapWidget(t, tt.edit)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "widget") {
				t.Errorf("err = %v, want one naming widget and %q", err, tt.wantErr)
			}
		})
	}
}

// Review Focus 2: an attribute that can be null can't map to a pointer on
// update, because a pointer can't carry null.
func TestMapRejectsNullablePointerUpdate(t *testing.T) {
	_, err := mapWidget(t, func(rc *ResourceConfig) {
		// color is Optional and nullable; a pointer field can't send its null.
		rc.API.UpdateParams = "WidgetPointerUpdateParams"
	})
	if err == nil || !strings.Contains(err.Error(), "color") {
		t.Errorf("err = %v, want one naming color", err)
	}
}

func TestMapNestedConverters(t *testing.T) {
	m, err := mapWidget(t, nil)
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
	if len(m.Nested) != 2 {
		t.Fatalf("Nested = %+v, want gear and gear.mesh", m.Nested)
	}
	gear, mesh := m.Nested[0], m.Nested[1]
	if gear.Prefix != "widgetGear" || gear.GoType != "Gear" || gear.ReadType != "Gear" {
		t.Errorf("gear = %+v", gear)
	}
	wantTo := []string{
		"setEnum(&out.Kind, m.Kind)",
		`out.Mesh = orZero(widgetGearMeshToAPI(m.Mesh, at.AtName("mesh"), diags))`,
	}
	wantFrom := []string{
		`"kind": types.StringValue(string(c.Kind)),`,
		`"mesh": widgetGearMeshFromAPI(&c.Mesh),`,
	}
	if !reflect.DeepEqual(gear.ToBody, wantTo) || !reflect.DeepEqual(gear.FromBody, wantFrom) {
		t.Errorf("gear bodies:\n to   %q\n from %q", gear.ToBody, gear.FromBody)
	}
	wantMeshTo := []string{
		"out.Pitch = m.Pitch.ValueFloat64()",
		"out.Teeth = int(m.Teeth.ValueInt64())",
	}
	if mesh.Prefix != "widgetGearMesh" || !reflect.DeepEqual(mesh.ToBody, wantMeshTo) {
		t.Errorf("mesh = %+v", mesh)
	}
}
