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
	Shape string // value, pointer, nullable, enum, enum_pointer, time or unsupported
	Elem  string // string, int, bool or float64, for value, pointer and nullable
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
		f, ok := shapeOf(st.Field(i).Type())
		if !ok {
			f = GoField{Shape: "unsupported"}
		}
		f.Name = st.Field(i).Name()
		out[name] = f
	}
	return out, nil
}

func shapeOf(t types.Type) (GoField, bool) {
	t = types.Unalias(t)
	switch t := t.(type) {
	case *types.Basic:
		if e, ok := basicElem(t); ok {
			return GoField{Shape: "value", Elem: e}, true
		}
	case *types.Pointer:
		inner, ok := shapeOf(t.Elem())
		switch {
		case ok && inner.Shape == "value":
			return GoField{Shape: "pointer", Elem: inner.Elem}, true
		case ok && inner.Shape == "enum":
			return GoField{Shape: "enum_pointer"}, true
		}
	case *types.Named:
		obj := t.Obj()
		pkg := ""
		if obj.Pkg() != nil {
			pkg = obj.Pkg().Path()
		}
		switch {
		case pkg == "time" && obj.Name() == "Time":
			return GoField{Shape: "time"}, true
		case pkg == "github.com/oapi-codegen/nullable" && obj.Name() == "Nullable":
			if t.TypeArgs().Len() == 1 {
				if b, ok := types.Unalias(t.TypeArgs().At(0)).(*types.Basic); ok {
					if e, ok := basicElem(b); ok {
						return GoField{Shape: "nullable", Elem: e}, true
					}
				}
			}
			return GoField{}, false
		}
		if b, ok := t.Underlying().(*types.Basic); ok && b.Kind() == types.String {
			return GoField{Shape: "enum"}, true
		}
	}
	return GoField{}, false
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
		m.Params = append(m.Params, GoParam{Name: p.Name(), Type: types.TypeString(p.Type(), qualify)})
	}
	if sig.Results().Len() > 0 {
		m.Result = types.TypeString(sig.Results().At(0).Type(), qualify)
	}
	return m, nil
}
