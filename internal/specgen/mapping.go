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
	IDUUID                           bool   // the id is a UUID, parsed from state before each call
	NameAttr                         string // model field naming the object in errors, or ""
	FindHint, Validators             string
	Parents                          []string
	CreateBody, UpdateBody, ReadBody []string
	Wrapper                          bool           // generate the CRUD wrapper
	Period                           bool           // the model uses customtypes
	Nested                           []NestedObject // converters for object attributes, parents first
	Maps                             []MapObject    // converters for map-backed object attributes
	Variant                          *VariantInfo   // the discriminator a variant resource sends and checks
	VariantRead                      string         // the read model's discriminator field
	Entry                            string         // the config entry, naming the variant names map
	Provider                         string         // the provider's type-name prefix, e.g. honeybadger
}

// MapObject is a map-backed object attribute's converters (phase 4b spec,
// 3.2): ToMap applies the one write rule; FromMap reads the map, carrying
// write-only keys from the prior value.
type MapObject struct {
	Prefix   string
	ToBody   []string // put… lines
	FromBody []string // map entries
}

// NestedObject is one object attribute's converters (phase 4a spec, 3.1).
type NestedObject struct {
	Prefix   string // names its model, attribute types and converters
	Path     string // e.g. trigger_config.config, for comments
	GoType   string // the struct a request carries
	ReadType string // the struct a response carries
	ToBody   []string
	FromBody []string // map entries: "name": expression,
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
		Wrapper:  rc.Service != "" && rc.CRUD != "manual",
		Provider: rc.Provider,
	}
	if rc.Entry != "" {
		m.Entry = lowerCamel(rc.Entry)
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
		prefix := nestedPrefix(lowerCamel(r.Name), a.Name)
		at := fmt.Sprintf("path.Root(%q)", a.Name)
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
			line, err := createLine(a, gf, "m."+field+suffix, "p", at, prefix)
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
			line, err := updateLine(a, gf, "m."+field+suffix, "state."+field+suffix, at, prefix)
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
		expr, err := readExpr(a, gf, "c", prefix)
		if err != nil {
			return fail("%v", err)
		}
		m.ReadBody = append(m.ReadBody, fmt.Sprintf("m.%s = %s", field, expr))
		switch {
		case a.Kind == "object" && (gf.Shape == "map" || gf.Shape == "map_pointer"):
			if err := addMap(m, a, prefix); err != nil {
				return fail("%v", err)
			}
		case a.Kind == "object", a.Kind == "list" && a.ElemKind == "object":
			toType := create[a.Name].Type
			if u, ok := update[a.Name]; ok && u.Type != "" && u.Type != toType {
				return fail("%s: create sends %s but update sends %s; specgen needs one struct for both", a.Name, toType, u.Type)
			}
			if err := addNested(api, m, a, prefix, a.Name, toType, gf.Type); err != nil {
				return fail("%v", err)
			}
		}
		if a.Name == "id" {
			m.IDField = gf.Name
			m.IDUUID = gf.Shape == "uuid"
		}
	}

	if r.Variant != nil {
		gf, ok := create[r.Variant.Field]
		if !ok {
			return fail("%s has no field in %s.%s", r.Variant.Field, api.Name, rc.API.CreateParams)
		}
		m.CreateBody = append(m.CreateBody, fmt.Sprintf("setEnum(&p.%s, types.StringValue(%q))", gf.Name, r.Variant.Const))
		rf, ok := read[r.Variant.Field]
		if !ok {
			return fail("%s has no field in %s.%s", r.Variant.Field, api.Name, rc.API.Model)
		}
		m.VariantRead = rf.Name
		m.Variant = r.Variant
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

// elemConv names the helpers that convert a collection's elements. Only
// strings are needed today; other elements fail generation until they are.
func elemConv(elem string) (from, to string, ok bool) {
	switch elem {
	case "string", "enum":
		return "fromString", "stringElem", true
	case "uuid":
		return "fromUUID", "uuidElem", true
	}
	return "", "", false
}

func createLine(a Attribute, gf GoField, v, dst, at, prefix string) (string, error) {
	conv := fmt.Sprintf("%sToAPI(%s, %s, diags)", prefix, v, at)
	from, _, elemOK := elemConv(gf.Elem)
	if strings.Contains(gf.Shape, "slice") && gf.Elem != "struct" && !elemOK {
		return "", fmt.Errorf("%s: slices of %s aren't mapped yet; extend specgen", a.Name, gf.Elem)
	}
	switch gf.Shape {
	case "value":
		if !a.Required {
			return "", fmt.Errorf("%s is a plain %s in the create params but isn't Required, so it would be sent as a zero value", a.Name, gf.Elem)
		}
		return fmt.Sprintf("%s.%s = %s", dst, gf.Name, valueOf(gf.Elem, v)), nil
	case "pointer":
		switch gf.Elem {
		case "string":
			return fmt.Sprintf("%s.%s = createStringPtr(%s)", dst, gf.Name, v), nil
		case "bool":
			return fmt.Sprintf("%s.%s = createBoolPtr(%s)", dst, gf.Name, v), nil
		}
	case "map_pointer":
		return fmt.Sprintf("%s.%s = %sToMap(%s, types.ObjectNull(%sAttrTypes()), %s, diags)", dst, gf.Name, prefix, v, prefix, at), nil
	case "nullable_string_map":
		return fmt.Sprintf("createStringMap(&%s.%s, %s)", dst, gf.Name, v), nil
	case "int_enum":
		if !a.Required {
			return "", fmt.Errorf("%s is a plain integer enum in the create params but isn't Required, so it would be sent as 0", a.Name)
		}
		return fmt.Sprintf("setIntEnum(&%s.%s, %s)", dst, gf.Name, v), nil
	case "nullable_int_enum":
		return fmt.Sprintf("createIntEnum(&%s.%s, %s)", dst, gf.Name, v), nil
	case "nullable_enum":
		return fmt.Sprintf("createEnum(&%s.%s, %s)", dst, gf.Name, v), nil
	case "nullable":
		if h, ok := map[string]string{"string": "createString", "int": "createInt", "bool": "createBool"}[gf.Elem]; ok {
			return fmt.Sprintf("%s.%s = %s(%s)", dst, gf.Name, h, v), nil
		}
	case "uuid":
		if !a.Required {
			return "", fmt.Errorf("%s is a plain UUID in the create params but isn't Required, so it would be sent as the zero UUID", a.Name)
		}
		return fmt.Sprintf("%s.%s = fromUUID(%s)", dst, gf.Name, v), nil
	case "enum":
		if !a.Required {
			return "", fmt.Errorf("%s is a plain enum in the create params but isn't Required, so it would be sent as an empty string", a.Name)
		}
		return fmt.Sprintf("setEnum(&%s.%s, %s)", dst, gf.Name, v), nil
	case "enum_pointer":
		return fmt.Sprintf("setEnumPtr(&%s.%s, %s)", dst, gf.Name, v), nil
	case "struct":
		if !a.Required {
			return "", fmt.Errorf("%s is a plain struct in the create params but isn't Required, so it would be sent as a zero value", a.Name)
		}
		return fmt.Sprintf("%s.%s = orZero(%s)", dst, gf.Name, conv), nil
	case "struct_pointer":
		return fmt.Sprintf("%s.%s = %s", dst, gf.Name, conv), nil
	case "nullable_struct":
		return fmt.Sprintf("%s.%s = nullableOf(%s)", dst, gf.Name, conv), nil
	case "slice":
		return "", fmt.Errorf("%s is a plain slice in the create params; only a pointer or nullable slice is mapped", a.Name)
	case "slice_pointer":
		if gf.Elem == "struct" {
			return "", fmt.Errorf("%s: a pointer to a slice of structs isn't mapped; only a nullable one is", a.Name)
		}
		return fmt.Sprintf("setSlicePtr(&%s.%s, %s, %s)", dst, gf.Name, v, from), nil
	case "nullable_slice":
		if gf.Elem == "struct" {
			return fmt.Sprintf("createNullableObjects(&%s.%s, %s, %sToAPI, %s, diags)", dst, gf.Name, v, prefix, at), nil
		}
		return fmt.Sprintf("createNullableSlice(&%s.%s, %s, %s)", dst, gf.Name, v, from), nil
	}
	return "", fmt.Errorf("%s: specgen doesn't map a %s create field", a.Name, describeShape(gf))
}

func updateLine(a Attribute, gf GoField, plan, state, at, prefix string) (string, error) {
	canBeNull := a.Optional && !a.Computed
	from, _, elemOK := elemConv(gf.Elem)
	if strings.Contains(gf.Shape, "slice") && gf.Elem != "struct" && !elemOK {
		return "", fmt.Errorf("%s: slices of %s aren't mapped yet; extend specgen", a.Name, gf.Elem)
	}
	switch gf.Shape {
	case "nullable":
		if h, ok := map[string]string{"string": "updateString", "int": "updateInt", "bool": "updateBool"}[gf.Elem]; ok {
			return fmt.Sprintf("p.%s = %s(%s, %s)", gf.Name, h, plan, state), nil
		}
	case "nullable_string_map":
		return fmt.Sprintf("updateStringMap(&p.%s, %s, %s)", gf.Name, plan, state), nil
	case "nullable_int_enum":
		return fmt.Sprintf("updateIntEnum(&p.%s, %s, %s)", gf.Name, plan, state), nil
	case "nullable_enum":
		return fmt.Sprintf("updateEnum(&p.%s, %s, %s)", gf.Name, plan, state), nil
	case "nullable_struct":
		return fmt.Sprintf("p.%s = updateNullableObject(%s, %s, %sToAPI(%s, %s, diags))", gf.Name, plan, state, prefix, plan, at), nil
	case "nullable_slice":
		if gf.Elem == "struct" {
			return fmt.Sprintf("updateNullableObjects(&p.%s, %s, %s, %sToAPI, %s, diags)", gf.Name, plan, state, prefix, at), nil
		}
		return fmt.Sprintf("updateNullableSlice(&p.%s, %s, %s, %s)", gf.Name, plan, state, from), nil
	case "map_pointer":
		if canBeNull {
			return "", fmt.Errorf("%s can be removed from config, but the update params' map pointer can't send null to clear it", a.Name)
		}
		return fmt.Sprintf("p.%s = changedPtr(%s, %s, %sToMap(%s, %s, %s, diags))", gf.Name, plan, state, prefix, plan, state, at), nil
	case "pointer", "enum_pointer", "struct_pointer", "slice_pointer":
		if canBeNull {
			return "", fmt.Errorf("%s can be removed from config, but the update params' pointer field can't send null to clear it", a.Name)
		}
		switch gf.Shape {
		case "enum_pointer":
			return fmt.Sprintf("updateEnumPtr(&p.%s, %s, %s)", gf.Name, plan, state), nil
		case "struct_pointer":
			return fmt.Sprintf("p.%s = changedPtr(%s, %s, %sToAPI(%s, %s, diags))", gf.Name, plan, state, prefix, plan, at), nil
		case "slice_pointer":
			return fmt.Sprintf("updateSlicePtr(&p.%s, %s, %s, %s)", gf.Name, plan, state, from), nil
		}
		switch gf.Elem {
		case "string":
			return fmt.Sprintf("p.%s = updateStringPtr(%s, %s)", gf.Name, plan, state), nil
		case "bool":
			return fmt.Sprintf("p.%s = updateBoolPtr(%s, %s)", gf.Name, plan, state), nil
		}
	case "value", "enum", "int_enum", "struct", "slice":
		return "", fmt.Errorf("%s is a plain value in the update params, so an update can't leave it unchanged", a.Name)
	}
	return "", fmt.Errorf("%s: specgen doesn't map a %s update field", a.Name, describeShape(gf))
}

// readExpr is the expression that reads a field of recv into an attribute.
func readExpr(a Attribute, gf GoField, recv, prefix string) (string, error) {
	x := recv + "." + gf.Name
	if a.CustomType == "period" {
		switch {
		case gf.Shape == "value" && gf.Elem == "string":
			return fmt.Sprintf("customtypes.NewPeriodValue(%s)", x), nil
		case gf.Shape == "nullable" && gf.Elem == "string":
			return fmt.Sprintf("periodValue(%s)", x), nil
		}
		return "", fmt.Errorf("%s: a period needs a string field, got %s", a.Name, describeShape(gf))
	}
	coll := "setOf"
	if a.Kind == "list" {
		coll = "listOf"
	}
	_, to, elemOK := elemConv(gf.Elem)
	if strings.Contains(gf.Shape, "slice") && gf.Elem != "struct" && !elemOK {
		return "", fmt.Errorf("%s: slices of %s aren't mapped yet; extend specgen", a.Name, gf.Elem)
	}
	elemType := ""
	if k, ok := kinds[a.ElemKind]; ok {
		elemType = k.elemType
	}
	switch gf.Shape {
	case "value":
		return map[string]string{
			"string": "types.StringValue(" + x + ")", "int": "types.Int64Value(int64(" + x + "))",
			"bool": "types.BoolValue(" + x + ")", "float64": "types.Float64Value(" + x + ")",
		}[gf.Elem], nil
	case "pointer":
		switch gf.Elem {
		case "string":
			return fmt.Sprintf("stringPtrValue(%s)", x), nil
		case "bool":
			return fmt.Sprintf("boolPtrValue(%s)", x), nil
		}
	case "nullable":
		if h, ok := map[string]string{"string": "stringValue", "int": "int64Value", "bool": "boolValue"}[gf.Elem]; ok {
			return fmt.Sprintf("%s(%s)", h, x), nil
		}
	case "enum":
		return fmt.Sprintf("types.StringValue(string(%s))", x), nil
	case "time":
		return fmt.Sprintf("timeValue(%s)", x), nil
	case "uuid":
		return fmt.Sprintf("types.StringValue(%s.String())", x), nil
	case "struct":
		return fmt.Sprintf("%sFromAPI(&%s)", prefix, x), nil
	case "struct_pointer":
		return fmt.Sprintf("%sFromAPI(%s)", prefix, x), nil
	case "nullable_struct":
		return fmt.Sprintf("%sFromAPI(nullablePtr(%s))", prefix, x), nil
	case "map":
		return fmt.Sprintf("%sFromMap(%s, m.%s)", prefix, x, upperCamel(a.Name)), nil
	case "nullable_string_map":
		return fmt.Sprintf("stringMapValue(%s)", x), nil
	case "string_map":
		return fmt.Sprintf("stringMapOf(%s)", x), nil
	case "int_enum":
		return fmt.Sprintf("types.Int64Value(int64(%s))", x), nil
	case "nullable_int_enum":
		return fmt.Sprintf("intEnumValue(%s)", x), nil
	case "nullable_enum":
		return fmt.Sprintf("enumValue(%s)", x), nil
	case "nullable_time":
		return fmt.Sprintf("nullableTimeValue(%s)", x), nil
	case "slice":
		if gf.Elem == "struct" {
			return fmt.Sprintf("objectList(%s, %sFromAPI, %sAttrTypes())", x, prefix, prefix), nil
		}
		return fmt.Sprintf("%s(%s, %s, %s)", coll, x, elemType, to), nil
	case "slice_pointer":
		return fmt.Sprintf("%s(derefSlice(%s), %s, %s)", coll, x, elemType, to), nil
	case "nullable_slice":
		return fmt.Sprintf("%s(nullableSlice(%s), %s, %s)", coll, x, elemType, to), nil
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
	switch {
	case gf.Type != "":
		return gf.Shape + " " + gf.Type
	case gf.Elem != "":
		return gf.Shape + " " + gf.Elem
	}
	return gf.Shape
}

// addNested builds an object's converters, then its child objects'. Every
// child is required (Merge refuses the rest), and the object is sent whole,
// so each child is written the way create writes it.
func addNested(api *API, m *Mapping, a Attribute, prefix, pathName, toType, fromType string) error {
	readOnly := a.Computed && !a.Optional
	if (toType == "" && !readOnly) || fromType == "" {
		return fmt.Errorf("%s: the client has no exported struct for it", pathName)
	}
	to := map[string]GoField{}
	if !readOnly {
		var err error
		if to, err = api.Fields(toType); err != nil {
			return err
		}
	}
	from, err := api.Fields(fromType)
	if err != nil {
		return err
	}
	n := NestedObject{Prefix: prefix, Path: pathName, GoType: toType, ReadType: fromType}
	type child struct {
		a        Attribute
		to, from GoField
	}
	var objects []child
	for _, c := range a.Attributes {
		field := upperCamel(c.Name)
		cp := nestedPrefix(prefix, c.Name)
		if c.NotRead {
			return fmt.Errorf("%s.%s: a write-only key in a struct-backed object can't be carried from the prior value", pathName, c.Name)
		}
		var tf GoField
		if !readOnly {
			var ok bool
			if tf, ok = to[c.Name]; !ok {
				return fmt.Errorf("%s.%s has no field in %s.%s", pathName, c.Name, api.Name, toType)
			}
			line, err := createLine(c, tf, "m."+field, "out", fmt.Sprintf("at.AtName(%q)", c.Name), cp)
			if err != nil {
				return fmt.Errorf("%s: %w", pathName, err)
			}
			n.ToBody = append(n.ToBody, line)
		}
		rf, ok := from[c.Name]
		if !ok {
			return fmt.Errorf("%s.%s has no field in %s.%s", pathName, c.Name, api.Name, fromType)
		}
		expr, err := readExpr(c, rf, "c", cp)
		if err != nil {
			return fmt.Errorf("%s: %w", pathName, err)
		}
		n.FromBody = append(n.FromBody, fmt.Sprintf("%q: %s,", c.Name, expr))
		if c.Kind == "object" {
			objects = append(objects, child{c, tf, rf})
		}
	}
	m.Nested = append(m.Nested, n)
	for _, o := range objects {
		if err := addNested(api, m, o.a, nestedPrefix(prefix, o.a.Name), pathName+"."+o.a.Name, o.to.Type, o.from.Type); err != nil {
			return err
		}
	}
	return nil
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
			isID := name != "Create" && i == len(w.params)-1
			typeOK := gp.Type == "string" || (isID && m.IDUUID && gp.UUID)
			if !typeOK || normalize(gp.Name) != normalize(p) {
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

func addMap(m *Mapping, a Attribute, prefix string) error {
	mo := MapObject{Prefix: prefix}
	for _, c := range a.Attributes {
		field := upperCamel(c.Name)
		put, read := "", ""
		switch c.Kind {
		case "string":
			put, read = "putString", fmt.Sprintf("mapString(m, %q)", c.Name)
		case "bool":
			put, read = "putBool", fmt.Sprintf("mapBool(m, %q)", c.Name)
		case "list", "set":
			if c.ElemKind != "string" {
				return fmt.Errorf("%s.%s: only lists of strings are mapped inside a map-backed object", a.Name, c.Name)
			}
			put, read = "putStrings", fmt.Sprintf("mapStrings(m, %q, %t)", c.Name, c.Kind == "set")
		default:
			return fmt.Errorf("%s.%s: a %s inside a map-backed object isn't mapped yet", a.Name, c.Name, c.Kind)
		}
		mo.ToBody = append(mo.ToBody, fmt.Sprintf("%s(out, %q, p.%s, s.%s)", put, c.Name, field, field))
		if c.NotRead {
			read = fmt.Sprintf("priorAttr(prior, %q, %s)", c.Name, nullOf(c))
		}
		mo.FromBody = append(mo.FromBody, fmt.Sprintf("%q: %s,", c.Name, read))
	}
	m.Maps = append(m.Maps, mo)
	return nil
}

// nullOf is an attribute's typed null.
func nullOf(a Attribute) string {
	switch a.Kind {
	case "list":
		return "types.ListNull(" + kinds[a.ElemKind].elemType + ")"
	case "set":
		return "types.SetNull(" + kinds[a.ElemKind].elemType + ")"
	case "map":
		return "types.MapNull(" + kinds[a.ElemKind].elemType + ")"
	}
	return map[string]string{"string": "types.StringNull()", "bool": "types.BoolNull()",
		"int64": "types.Int64Null()", "float64": "types.Float64Null()"}[a.Kind]
}
