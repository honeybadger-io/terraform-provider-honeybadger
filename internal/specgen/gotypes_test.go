package specgen

import (
	"reflect"
	"strings"
	"testing"
)

const fakeAPIPath = "github.com/honeybadger-io/terraform-provider-honeybadger/internal/specgen/testdata/fakeapi"

func loadFakeAPI(t *testing.T) *API {
	t.Helper()
	api, err := LoadAPI(".", fakeAPIPath)
	if err != nil {
		t.Fatalf("LoadAPI: %v", err)
	}
	return api
}

func TestFieldsClassifyShapes(t *testing.T) {
	read, err := loadFakeAPI(t).Fields("Widget")
	if err != nil {
		t.Fatalf("Fields: %v", err)
	}
	want := map[string]GoField{
		"id":         {Name: "Id", Shape: "value", Elem: "string"},
		"shape":      {Name: "Shape", Shape: "enum"},
		"size":       {Name: "Size", Shape: "nullable", Elem: "int"},
		"color":      {Name: "Color", Shape: "nullable", Elem: "string"},
		"tags":       {Name: "Tags", Shape: "slice", Elem: "string"},
		"gear":       {Name: "Gear", Shape: "nullable_struct", Type: "Gear"},
		"parts":      {Name: "Parts", Shape: "nullable_slice", Elem: "string"},
		"serial":     {Name: "Serial", Shape: "pointer", Elem: "string"},
		"created_at": {Name: "CreatedAt", Shape: "time"},
	}
	for name, w := range want {
		if got := read[name]; !reflect.DeepEqual(got, w) {
			t.Errorf("%s = %+v, want %+v", name, got, w)
		}
	}
	create, err := loadFakeAPI(t).Fields("WidgetCreateParams")
	if err != nil {
		t.Fatalf("Fields: %v", err)
	}
	if got := create["shape"]; got.Shape != "enum_pointer" {
		t.Errorf("create shape = %+v, want enum_pointer", got)
	}
}

func TestFieldsUnknownType(t *testing.T) {
	_, err := loadFakeAPI(t).Fields("Gizmo")
	if err == nil || !strings.Contains(err.Error(), "Gizmo") {
		t.Errorf("err = %v, want one naming Gizmo", err)
	}
}

func TestMethodSignatures(t *testing.T) {
	m, err := loadFakeAPI(t).Method("Widgets", "Update")
	if err != nil {
		t.Fatalf("Method: %v", err)
	}
	want := GoMethod{Params: []GoParam{{"projectID", "string"}, {"widgetID", "string"}, {"p", "WidgetUpdateParams"}}, Result: "*Widget"}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("Update = %+v, want %+v", m, want)
	}
	del, err := loadFakeAPI(t).Method("Widgets", "Delete")
	if err != nil || del.Result != "error" {
		t.Errorf("Delete = %+v, %v; want result error", del, err)
	}
	if _, err := loadFakeAPI(t).Method("WidgetsNoDelete", "Delete"); err == nil || !strings.Contains(err.Error(), "Delete") {
		t.Errorf("missing method err = %v", err)
	}
}

// The real apiv3 loads through go.work and has the shapes 2b relies on.
func TestRealAPIShapes(t *testing.T) {
	api, err := LoadAPI(".", "github.com/honeybadger-io/api-go/apiv3")
	if err != nil {
		t.Fatalf("LoadAPI: %v", err)
	}
	upd, err := api.Fields("CheckInUpdateParams")
	if err != nil {
		t.Fatalf("Fields: %v", err)
	}
	if upd["schedule_type"].Shape != "enum_pointer" || upd["grace_period"].Shape != "nullable" {
		t.Errorf("check-in update shapes = %+v", upd)
	}
	pp, err := api.Fields("ProjectParams")
	if err != nil || pp["name"].Shape != "pointer" {
		t.Errorf("ProjectParams.name = %+v, %v; want pointer", pp["name"], err)
	}
}

func TestFieldsClassifyWriteShapes(t *testing.T) {
	api := loadFakeAPI(t)
	create, err := api.Fields("WidgetCreateParams")
	if err != nil {
		t.Fatal(err)
	}
	update, err := api.Fields("WidgetUpdateParams")
	if err != nil {
		t.Fatal(err)
	}
	mesh, err := api.Fields("Mesh")
	if err != nil {
		t.Fatal(err)
	}
	for name, pair := range map[string][2]GoField{
		"create gear":  {create["gear"], {Name: "Gear", Shape: "struct", Type: "Gear"}},
		"create parts": {create["parts"], {Name: "Parts", Shape: "slice_pointer", Elem: "string"}},
		"update gear":  {update["gear"], {Name: "Gear", Shape: "struct_pointer", Type: "Gear"}},
		"mesh pitch":   {mesh["pitch"], {Name: "Pitch", Shape: "value", Elem: "float64"}},
	} {
		if !reflect.DeepEqual(pair[0], pair[1]) {
			t.Errorf("%s = %+v, want %+v", name, pair[0], pair[1])
		}
	}
}
