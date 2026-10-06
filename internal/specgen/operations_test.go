package specgen

import (
	"reflect"
	"strings"
	"testing"
)

func TestFindOperationUnwrapsTheEnvelope(t *testing.T) {
	op, err := FindOperation(loadFixture(t), "createWidget")
	if err != nil {
		t.Fatalf("FindOperation: %v", err)
	}
	if op.Method != "POST" || op.Path != "/projects/{project_id}/widgets" {
		t.Errorf("method/path = %s %s", op.Method, op.Path)
	}
	if want := []Param{{Name: "project_id", Description: "The project's ID."}}; !reflect.DeepEqual(op.PathParams, want) {
		t.Errorf("path params = %+v, want %+v", op.PathParams, want)
	}
	if op.Request == nil || op.Request.Name != "WidgetCreateInput" {
		t.Errorf("request = %+v, want WidgetCreateInput", op.Request)
	}
	if op.Response == nil || op.Response.Name != "Widget" {
		t.Errorf("response = %+v, want the data schema Widget", op.Response)
	}
}

func TestFindOperationPathParamsInOrder(t *testing.T) {
	op, err := FindOperation(loadFixture(t), "getWidget")
	if err != nil {
		t.Fatalf("FindOperation: %v", err)
	}
	var names []string
	for _, p := range op.PathParams {
		names = append(names, p.Name)
	}
	if !reflect.DeepEqual(names, []string{"project_id", "widget_id"}) {
		t.Errorf("path params = %v", names)
	}
}

func TestFindOperationWithoutBodies(t *testing.T) {
	doc := loadFixture(t)
	del, err := FindOperation(doc, "deleteWidget")
	if err != nil {
		t.Fatalf("FindOperation: %v", err)
	}
	if del.Request != nil || del.Response != nil {
		t.Errorf("delete = %+v, want no request or response schema", del)
	}
	list, err := FindOperation(doc, "listWidgets")
	if err != nil {
		t.Fatalf("FindOperation: %v", err)
	}
	if list.Response != nil {
		t.Errorf("list response = %+v, want nil (data is an array)", list.Response)
	}
}

func TestFindOperationUnknown(t *testing.T) {
	_, err := FindOperation(loadFixture(t), "frobnicateWidget")
	if err == nil || !strings.Contains(err.Error(), "frobnicateWidget") {
		t.Errorf("err = %v, want one naming the operation", err)
	}
}

func TestFindOperationRealBundle(t *testing.T) {
	doc, err := Load("../../openapi/bundled.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	op, err := FindOperation(doc, "createCheckIn")
	if err != nil {
		t.Fatalf("FindOperation: %v", err)
	}
	if op.Request == nil || op.Request.Name != "CheckInCreateInput" || op.Response == nil || op.Response.Name != "CheckIn" {
		t.Errorf("createCheckIn = request %v, response %v", op.Request, op.Response)
	}
}
