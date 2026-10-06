package specgen

import (
	"fmt"
	"go/types"
	"reflect"
	"strings"

	"golang.org/x/tools/go/packages"
)

// API is a loaded client package, such as apiv3, read with go/types so the
// mapping uses its real field names and types (spec 2b, section 3.2).
type API struct {
	Name string // package name, e.g. apiv3
	Path string // import path
	pkg  *types.Package
}

// LoadAPI loads the package at path, resolving modules from dir (so go.work
// picks the local api-go).
func LoadAPI(dir, path string) (*API, error) {
	pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedName | packages.NeedTypes, Dir: dir}, path)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", path, err)
	}
	if len(pkgs) != 1 {
		return nil, fmt.Errorf("loading %s: got %d packages", path, len(pkgs))
	}
	if len(pkgs[0].Errors) > 0 {
		return nil, fmt.Errorf("loading %s: %v", path, pkgs[0].Errors)
	}
	return &API{Name: pkgs[0].Types.Name(), Path: path, pkg: pkgs[0].Types}, nil
}

// GoField is one struct field, found by its JSON name.
type GoField struct {
	Name  string // Go field name
	Shape string // value, pointer, nullable, enum, enum_pointer, time, uuid, struct, struct_pointer,
	// nullable_struct, slice, slice_pointer, nullable_slice, map, map_pointer or unsupported
	Elem string // string, int, bool or float64, for value, pointer, nullable and slice shapes
	Type string // for struct shapes: the API package's exported name for the struct
}

// Fields returns a named struct's fields keyed by JSON name.
func (a *API) Fields(typeName string) (map[string]GoField, error) {
	obj := a.pkg.Scope().Lookup(typeName)
	if obj == nil {
		return nil, fmt.Errorf("%s.%s doesn't exist", a.Name, typeName)
	}
	st, ok := obj.Type().Underlying().(*types.Struct)
	if !ok {
		return nil, fmt.Errorf("%s.%s isn't a struct", a.Name, typeName)
	}
	out := map[string]GoField{}
	for i := 0; i < st.NumFields(); i++ {
		name := strings.Split(reflect.StructTag(st.Tag(i)).Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		f, ok := a.shapeOf(st.Field(i).Type())
		if !ok {
			f = GoField{Shape: "unsupported"}
		}
		f.Name = st.Field(i).Name()
		out[name] = f
	}
	return out, nil
}

func (a *API) shapeOf(t types.Type) (GoField, bool) {
	t = types.Unalias(t)
	switch t := t.(type) {
	case *types.Basic:
		if e, ok := basicElem(t); ok {
			return GoField{Shape: "value", Elem: e}, true
		}
	case *types.Slice:
		if e, tn, ok := a.elemOf(t.Elem()); ok {
			return GoField{Shape: "slice", Elem: e, Type: tn}, true
		}
	case *types.Map:
		k, kok := types.Unalias(t.Key()).(*types.Basic)
		v, vok := types.Unalias(t.Elem()).Underlying().(*types.Interface)
		if kok && k.Kind() == types.String && vok && v.Empty() {
			return GoField{Shape: "map"}, true
		}
	case *types.Pointer:
		inner, ok := a.shapeOf(t.Elem())
		switch {
		case ok && inner.Shape == "value":
			return GoField{Shape: "pointer", Elem: inner.Elem}, true
		case ok && inner.Shape == "enum":
			return GoField{Shape: "enum_pointer"}, true
		case ok && inner.Shape == "struct":
			return GoField{Shape: "struct_pointer", Type: inner.Type}, true
		case ok && inner.Shape == "slice":
			return GoField{Shape: "slice_pointer", Elem: inner.Elem, Type: inner.Type}, true
		case ok && inner.Shape == "map":
			return GoField{Shape: "map_pointer"}, true
		}
	case *types.Named:
		obj := t.Obj()
		pkg := ""
		if obj.Pkg() != nil {
			pkg = obj.Pkg().Path()
		}
		switch {
		case isUUID(t):
			return GoField{Shape: "uuid"}, true
		case pkg == "time" && obj.Name() == "Time":
			return GoField{Shape: "time"}, true
		case pkg == "github.com/oapi-codegen/nullable" && obj.Name() == "Nullable":
			if t.TypeArgs().Len() != 1 {
				return GoField{}, false
			}
			inner, ok := a.shapeOf(t.TypeArgs().At(0))
			switch {
			case ok && inner.Shape == "value":
				return GoField{Shape: "nullable", Elem: inner.Elem}, true
			case ok && inner.Shape == "struct":
				return GoField{Shape: "nullable_struct", Type: inner.Type}, true
			case ok && inner.Shape == "slice":
				return GoField{Shape: "nullable_slice", Elem: inner.Elem, Type: inner.Type}, true
			}
			return GoField{}, false
		}
		switch u := t.Underlying().(type) {
		case *types.Basic:
			if u.Kind() == types.String {
				return GoField{Shape: "enum"}, true
			}
		case *types.Struct:
			if name, ok := a.exportedName(t); ok {
				return GoField{Shape: "struct", Type: name}, true
			}
		}
	}
	return GoField{}, false
}

// exportedName is the name generated code can use for a struct: its own name
// when the API package declares it, or an exported alias for it (apiv3
// re-exports api-go's internal types as aliases).
func (a *API) exportedName(t *types.Named) (string, bool) {
	if t.Obj().Pkg() == a.pkg {
		return t.Obj().Name(), t.Obj().Exported()
	}
	for _, n := range a.pkg.Scope().Names() {
		tn, ok := a.pkg.Scope().Lookup(n).(*types.TypeName)
		if ok && tn.Exported() && types.Identical(types.Unalias(tn.Type()), t) {
			return n, true
		}
	}
	return "", false
}

func basicElem(b *types.Basic) (string, bool) {
	switch b.Kind() {
	case types.String:
		return "string", true
	case types.Int, types.Int64:
		return "int", true
	case types.Bool:
		return "bool", true
	case types.Float64:
		return "float64", true
	}
	return "", false
}

// GoParam is a method parameter.
type GoParam struct {
	Name string
	Type string // unqualified for the API package's own types
	UUID bool   // a github.com/google/uuid UUID, under any alias
}

// isUUID reports whether t is github.com/google/uuid's UUID, under any alias
// (api-go's openapi_types.UUID and apiv3.SiteID are both aliases).
func isUUID(t types.Type) bool {
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "github.com/google/uuid" && n.Obj().Name() == "UUID"
}

// GoMethod is a service method's parameters after ctx, and its first result.
type GoMethod struct {
	Params []GoParam
	Result string
}

// Method finds Client.<service>.<method>.
func (a *API) Method(service, method string) (GoMethod, error) {
	client := a.pkg.Scope().Lookup("Client")
	if client == nil {
		return GoMethod{}, fmt.Errorf("%s has no Client", a.Name)
	}
	st, ok := client.Type().Underlying().(*types.Struct)
	if !ok {
		return GoMethod{}, fmt.Errorf("%s.Client isn't a struct", a.Name)
	}
	var svc types.Type
	for i := 0; i < st.NumFields(); i++ {
		if st.Field(i).Name() == service {
			svc = st.Field(i).Type()
		}
	}
	if svc == nil {
		return GoMethod{}, fmt.Errorf("%s.Client has no %s service", a.Name, service)
	}
	sel := types.NewMethodSet(svc).Lookup(a.pkg, method)
	if sel == nil {
		return GoMethod{}, fmt.Errorf("%s.%s has no %s method", a.Name, service, method)
	}
	sig := sel.Obj().Type().(*types.Signature)
	qualify := func(p *types.Package) string {
		if p == a.pkg {
			return ""
		}
		return p.Name()
	}
	var m GoMethod
	for i := 1; i < sig.Params().Len(); i++ {
		p := sig.Params().At(i)
		m.Params = append(m.Params, GoParam{Name: p.Name(), Type: types.TypeString(p.Type(), qualify), UUID: isUUID(p.Type())})
	}
	if sig.Results().Len() > 0 {
		m.Result = types.TypeString(sig.Results().At(0).Type(), qualify)
	}
	return m, nil
}

// elemOf classifies a slice element: a basic type, an enum (named ~string),
// a UUID (github.com/google/uuid), or an exported struct.
func (a *API) elemOf(t types.Type) (elem, typeName string, ok bool) {
	t = types.Unalias(t)
	if b, isBasic := t.(*types.Basic); isBasic {
		e, ok := basicElem(b)
		return e, "", ok
	}
	n, isNamed := t.(*types.Named)
	if !isNamed {
		return "", "", false
	}
	if isUUID(n) {
		return "uuid", "", true
	}
	switch u := n.Underlying().(type) {
	case *types.Basic:
		if u.Kind() == types.String {
			return "enum", "", true
		}
	case *types.Struct:
		if name, ok := a.exportedName(n); ok {
			return "struct", name, true
		}
	}
	return "", "", false
}
