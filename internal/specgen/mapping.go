package specgen

import (
	"fmt"
	"slices"
	"strings"
)

// Mapping is the generated glue between a resource's model and its client
// types: Go statements for createParams, updateParams and fromAPI, plus what
// the CRUD wrapper needs (spec 2b, sections 3.2 and 3.3).
type Mapping struct {
	PkgName, PkgPath                 string
	Service, Model                   string
	CreateParams, UpdateParams       string
	IDField                          string // the model's id field, e.g. Id
	NameAttr                         string // model field naming the object in errors, or ""
	FindHint, Validators             string
	Parents                          []string
	CreateBody, UpdateBody, ReadBody []string
	Wrapper                          bool // generate the CRUD wrapper
	Period                           bool // the model uses customtypes
}

// Map builds a resource's mapping against the client package's real types.
func Map(api *API, r Resource, rc ResourceConfig) (*Mapping, error) {
	fail := func(format string, args ...any) (*Mapping, error) {
		return nil, fmt.Errorf("resource %s: "+format, append([]any{r.Name}, args...)...)
	}
	m := &Mapping{
		PkgName: api.Name, PkgPath: api.Path, Service: rc.Service, Model: rc.API.Model,
		CreateParams: rc.API.CreateParams, UpdateParams: rc.API.UpdateParams,
		FindHint: rc.FindHint, Validators: rc.ConfigValidators, Parents: r.Parents,
		Wrapper: rc.Service != "" && rc.CRUD != "manual",
	}
	if m.Wrapper && r.Import == "" {
		return fail("a generated wrapper needs an import format")
	}
	read, err := api.Fields(rc.API.Model)
	if err != nil {
		return fail("%v", err)
	}
	create, err := api.Fields(rc.API.CreateParams)
	if err != nil {
		return fail("%v", err)
	}
	update := map[string]GoField{}
	if rc.API.UpdateParams != "" {
		if update, err = api.Fields(rc.API.UpdateParams); err != nil {
			return fail("%v", err)
		}
	}

	for _, a := range r.Attributes {
		field := upperCamel(a.Name)
		writable := a.Required || a.Optional
		parent := slices.Contains(r.Parents, a.Name)
		suffix := ""
		if a.CustomType != "" {
			suffix = ".StringValue"
			m.Period = true
		}

		if writable && !parent {
			gf, ok := create[a.Name]
			if !ok {
				return fail("%s has no field in %s.%s", a.Name, api.Name, rc.API.CreateParams)
			}
			line, err := createLine(a, gf, "m."+field+suffix)
			if err != nil {
				return fail("%v", err)
			}
			m.CreateBody = append(m.CreateBody, line)
		}

		if writable && !parent && !a.RequiresReplace && rc.API.UpdateParams != "" {
			gf, ok := update[a.Name]
			if !ok {
				return fail("%s has no field in %s.%s", a.Name, api.Name, rc.API.UpdateParams)
			}
			line, err := updateLine(a, gf, "m."+field+suffix, "state."+field+suffix)
			if err != nil {
				return fail("%v", err)
			}
			m.UpdateBody = append(m.UpdateBody, line)
		}

		if a.NotRead {
			continue
		}
		gf, ok := read[a.Name]
		if !ok {
			return fail("%s has no field in %s.%s", a.Name, api.Name, rc.API.Model)
		}
		line, err := readLine(a, gf, field)
		if err != nil {
			return fail("%v", err)
		}
		m.ReadBody = append(m.ReadBody, line)
		if a.Name == "id" {
			m.IDField = gf.Name
		}
	}

	for _, n := range []string{"name", "label"} {
		if slices.ContainsFunc(r.Attributes, func(a Attribute) bool { return a.Name == n && a.Kind == "string" }) {
			m.NameAttr = upperCamel(n)
			break
		}
	}

	if m.Wrapper {
		if err := checkService(api, m, r); err != nil {
			return fail("%v", err)
		}
	}
	return m, nil
}

func createLine(a Attribute, gf GoField, v string) (string, error) {
	switch gf.Shape {
	case "value":
		if !a.Required {
			return "", fmt.Errorf("%s is a plain %s in the create params but isn't Required, so it would be sent as a zero value", a.Name, gf.Elem)
		}
		return fmt.Sprintf("p.%s = %s", gf.Name, valueOf(gf.Elem, v)), nil
	case "pointer":
		if gf.Elem == "string" {
			return fmt.Sprintf("p.%s = createStringPtr(%s)", gf.Name, v), nil
		}
	case "nullable":
		if h, ok := map[string]string{"string": "createString", "int": "createInt", "bool": "createBool"}[gf.Elem]; ok {
			return fmt.Sprintf("p.%s = %s(%s)", gf.Name, h, v), nil
		}
	case "enum":
		if !a.Required {
			return "", fmt.Errorf("%s is a plain enum in the create params but isn't Required, so it would be sent as an empty string", a.Name)
		}
		return fmt.Sprintf("setEnum(&p.%s, %s)", gf.Name, v), nil
	case "enum_pointer":
		return fmt.Sprintf("setEnumPtr(&p.%s, %s)", gf.Name, v), nil
	}
	return "", fmt.Errorf("%s: specgen doesn't map a %s create field", a.Name, describeShape(gf))
}

func updateLine(a Attribute, gf GoField, plan, state string) (string, error) {
	canBeNull := a.Optional && !a.Computed
	switch gf.Shape {
	case "nullable":
		if h, ok := map[string]string{"string": "updateString", "int": "updateInt", "bool": "updateBool"}[gf.Elem]; ok {
			return fmt.Sprintf("p.%s = %s(%s, %s)", gf.Name, h, plan, state), nil
		}
	case "pointer", "enum_pointer":
		if canBeNull {
			return "", fmt.Errorf("%s can be removed from config, but the update params' pointer field can't send null to clear it", a.Name)
		}
		if gf.Shape == "enum_pointer" {
			return fmt.Sprintf("updateEnumPtr(&p.%s, %s, %s)", gf.Name, plan, state), nil
		}
		if gf.Elem == "string" {
			return fmt.Sprintf("p.%s = updateStringPtr(%s, %s)", gf.Name, plan, state), nil
		}
	case "value", "enum":
		return "", fmt.Errorf("%s is a plain value in the update params, so an update can't leave it unchanged", a.Name)
	}
	return "", fmt.Errorf("%s: specgen doesn't map a %s update field", a.Name, describeShape(gf))
}

func readLine(a Attribute, gf GoField, field string) (string, error) {
	if a.CustomType == "period" {
		switch {
		case gf.Shape == "value" && gf.Elem == "string":
			return fmt.Sprintf("m.%s = customtypes.NewPeriodValue(c.%s)", field, gf.Name), nil
		case gf.Shape == "nullable" && gf.Elem == "string":
			return fmt.Sprintf("m.%s = periodValue(c.%s)", field, gf.Name), nil
		}
		return "", fmt.Errorf("%s: a period needs a string field, got %s", a.Name, describeShape(gf))
	}
	switch gf.Shape {
	case "value":
		return fmt.Sprintf("m.%s = %s", field, map[string]string{
			"string": "types.StringValue(c." + gf.Name + ")", "int": "types.Int64Value(int64(c." + gf.Name + "))",
			"bool": "types.BoolValue(c." + gf.Name + ")", "float64": "types.Float64Value(c." + gf.Name + ")",
		}[gf.Elem]), nil
	case "pointer":
		if gf.Elem == "string" {
			return fmt.Sprintf("m.%s = stringPtrValue(c.%s)", field, gf.Name), nil
		}
	case "nullable":
		if h, ok := map[string]string{"string": "stringValue", "int": "int64Value", "bool": "boolValue"}[gf.Elem]; ok {
			return fmt.Sprintf("m.%s = %s(c.%s)", field, h, gf.Name), nil
		}
	case "enum":
		return fmt.Sprintf("m.%s = types.StringValue(string(c.%s))", field, gf.Name), nil
	case "time":
		return fmt.Sprintf("m.%s = timeValue(c.%s)", field, gf.Name), nil
	}
	return "", fmt.Errorf("%s: specgen doesn't map a %s read field", a.Name, describeShape(gf))
}

func valueOf(elem, v string) string {
	return map[string]string{
		"string": v + ".ValueString()", "int": "int(" + v + ".ValueInt64())",
		"bool": v + ".ValueBool()", "float64": v + ".ValueFloat64()",
	}[elem]
}

func describeShape(gf GoField) string {
	if gf.Elem != "" {
		return gf.Shape + " " + gf.Elem
	}
	return gf.Shape
}

// checkService confirms the client's methods take the resource's path
// parameters in order, plus the configured params type, and return its model.
func checkService(api *API, m *Mapping, r Resource) error {
	ids := append(append([]string{}, r.Parents...), r.IDParam)
	type want struct {
		params     []string
		paramsType string
		result     string
	}
	methods := map[string]want{
		"Create": {r.Parents, m.CreateParams, "*" + m.Model},
		"Get":    {ids, "", "*" + m.Model},
		"Delete": {ids, "", "error"},
	}
	if m.UpdateParams != "" {
		methods["Update"] = want{ids, m.UpdateParams, "*" + m.Model}
	}
	for _, name := range []string{"Create", "Get", "Update", "Delete"} {
		w, ok := methods[name]
		if !ok {
			continue
		}
		got, err := api.Method(m.Service, name)
		if err != nil {
			return err
		}
		n := len(w.params)
		if w.paramsType != "" {
			n++
		}
		if len(got.Params) != n {
			return fmt.Errorf("%s.%s.%s takes %d parameters after ctx, want %d", api.Name, m.Service, name, len(got.Params), n)
		}
		for i, p := range w.params {
			gp := got.Params[i]
			if gp.Type != "string" || normalize(gp.Name) != normalize(p) {
				return fmt.Errorf("%s.%s.%s parameter %d is %s %s, but path parameter %d is %s", api.Name, m.Service, name, i+1, gp.Name, gp.Type, i+1, p)
			}
		}
		if w.paramsType != "" && got.Params[n-1].Type != w.paramsType {
			return fmt.Errorf("%s.%s.%s takes %s, want %s", api.Name, m.Service, name, got.Params[n-1].Type, w.paramsType)
		}
		if got.Result != w.result {
			return fmt.Errorf("%s.%s.%s returns %s, want %s", api.Name, m.Service, name, got.Result, w.result)
		}
	}
	return nil
}

// normalize compares path parameter names with Go names: check_in_id and
// checkInID both become checkinid.
func normalize(s string) string { return strings.ToLower(strings.ReplaceAll(s, "_", "")) }
