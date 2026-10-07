package specgen

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// Param is a path parameter.
type Param struct {
	Name        string
	Description string
}

// Operation is what a resource needs from one API operation.
type Operation struct {
	ID            string
	Method        string
	Path          string
	PathParams    []Param             // in path order
	Request       *Object             // nil without a JSON request body
	RequestSchema *openapi3.SchemaRef // the request body's schema, for unions
	Response      *Object             // the success response's data object; nil when there isn't one
	Scopes        []string            // the operation's security scopes, or the document's
}

var pathParam = regexp.MustCompile(`\{([^}]+)\}`)

// FindOperation looks an operation up by its operationId.
func FindOperation(doc *openapi3.T, id string) (Operation, error) {
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			if op.OperationID != id {
				continue
			}
			return describe(doc, path, method, item, op)
		}
	}
	return Operation{}, fmt.Errorf("operation %q isn't in the spec", id)
}

func describe(doc *openapi3.T, path, method string, item *openapi3.PathItem, op *openapi3.Operation) (Operation, error) {
	out := Operation{ID: op.OperationID, Method: strings.ToUpper(method), Path: path}

	reqs := doc.Security
	if op.Security != nil {
		reqs = *op.Security
	}
	seen := map[string]bool{}
	for _, req := range reqs {
		for _, scopes := range req {
			for _, s := range scopes {
				if !seen[s] {
					seen[s] = true
					out.Scopes = append(out.Scopes, s)
				}
			}
		}
	}
	sort.Strings(out.Scopes)

	described := map[string]string{}
	for _, params := range []openapi3.Parameters{item.Parameters, op.Parameters} {
		for _, p := range params {
			if p.Value != nil && p.Value.In == "path" {
				described[p.Value.Name] = p.Value.Description
			}
		}
	}
	for _, m := range pathParam.FindAllStringSubmatch(path, -1) {
		out.PathParams = append(out.PathParams, Param{Name: m[1], Description: described[m[1]]})
	}

	if op.RequestBody != nil && op.RequestBody.Value != nil {
		if media := op.RequestBody.Value.Content.Get("application/json"); media != nil && media.Schema != nil {
			obj, err := ObjectFacts(schemaName(media.Schema), media.Schema)
			if err != nil {
				return Operation{}, fmt.Errorf("%s request: %w", op.OperationID, err)
			}
			out.Request = &obj
			out.RequestSchema = media.Schema
		}
	}

	for code, resp := range op.Responses.Map() {
		if !strings.HasPrefix(code, "2") || resp.Value == nil {
			continue
		}
		media := resp.Value.Content.Get("application/json")
		if media == nil || media.Schema == nil || media.Schema.Value == nil {
			continue
		}
		data := media.Schema.Value.Properties["data"]
		if data == nil || data.Value == nil || data.Value.Type == nil || !data.Value.Type.Is("object") {
			continue
		}
		obj, err := ObjectFacts(schemaName(data), data)
		if err != nil {
			return Operation{}, fmt.Errorf("%s response: %w", op.OperationID, err)
		}
		out.Response = &obj
		break
	}
	return out, nil
}

// schemaName is a $ref's component name, or "inline".
func schemaName(ref *openapi3.SchemaRef) string {
	if i := strings.LastIndex(ref.Ref, "/"); i >= 0 {
		return ref.Ref[i+1:]
	}
	return "inline"
}
