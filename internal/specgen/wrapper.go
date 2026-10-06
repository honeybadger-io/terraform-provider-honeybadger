package specgen

import (
	"strings"
	"text/template"
)

// wrapperData is what the CRUD wrapper template needs.
type wrapperData struct {
	*Mapping
	Name, Camel, Upper, Human string
	CreateArgs, IDArgs        string // leading path arguments, each followed by ", "
	ProjectIDExpr, NameExpr   string
}

func newWrapperData(r Resource, m *Mapping) wrapperData {
	d := wrapperData{Mapping: m, Name: r.Name, Camel: lowerCamel(r.Name), Upper: upperCamel(r.Name),
		Human: strings.ReplaceAll(r.Name, "_", " "), ProjectIDExpr: `""`, NameExpr: `""`}
	for _, p := range r.Parents {
		d.CreateArgs += "plan." + upperCamel(p) + ".ValueString(), "
		d.IDArgs += "state." + upperCamel(p) + ".ValueString(), "
		if p == "project_id" {
			d.ProjectIDExpr = "plan.ProjectID.ValueString()"
		}
	}
	d.IDArgs += "state.ID.ValueString()"
	if m.NameAttr != "" {
		d.NameExpr = "plan." + m.NameAttr + ".ValueString()"
	}
	return d
}

var mappingTmpl = template.Must(template.New("mapping").Parse(`
func (m {{.Camel}}ResourceModel) createParams(diags *diag.Diagnostics) {{.PkgName}}.{{.CreateParams}} {
	var p {{.PkgName}}.{{.CreateParams}}
{{- range .CreateBody}}
	{{.}}
{{- end}}
	return p
}
{{if .UpdateParams}}
func (m {{.Camel}}ResourceModel) updateParams(state {{.Camel}}ResourceModel, diags *diag.Diagnostics) {{.PkgName}}.{{.UpdateParams}} {
	var p {{.PkgName}}.{{.UpdateParams}}
{{- range .UpdateBody}}
	{{.}}
{{- end}}
	return p
}
{{end}}
func (m *{{.Camel}}ResourceModel) fromAPI(c *{{.PkgName}}.{{.Model}}) {
{{- range .ReadBody}}
	{{.}}
{{- end}}
}
{{range .Nested}}
// {{.Prefix}}ToAPI converts {{.Path}} for a request. It's nil when the
// object is null or unknown, or holds a value that isn't known, which it
// reports: the API replaces the object whole.
func {{.Prefix}}ToAPI(o types.Object, at path.Path, diags *diag.Diagnostics) *{{$.PkgName}}.{{.GoType}} {
	var m {{.Prefix}}Model
	if !objectAs(o, &m, at, diags) {
		return nil
	}
	var out {{$.PkgName}}.{{.GoType}}
{{- range .ToBody}}
	{{.}}
{{- end}}
	return &out
}

// {{.Prefix}}FromAPI reads {{.Path}}: null when the API returns none.
func {{.Prefix}}FromAPI(c *{{$.PkgName}}.{{.ReadType}}) types.Object {
	if c == nil {
		return types.ObjectNull({{.Prefix}}AttrTypes())
	}
	return types.ObjectValueMust({{.Prefix}}AttrTypes(), map[string]attr.Value{
{{- range .FromBody}}
		{{.}}
{{- end}}
	})
}
{{end}}
`))

var wrapperTmpl = template.Must(template.New("wrapper").Parse(`
var (
	_ resource.Resource                = (*{{.Camel}}Resource)(nil)
	_ resource.ResourceWithConfigure   = (*{{.Camel}}Resource)(nil)
	_ resource.ResourceWithImportState = (*{{.Camel}}Resource)(nil)
{{- if .Validators}}
	_ resource.ResourceWithConfigValidators = (*{{.Camel}}Resource)(nil)
{{- end}}
)

// {{.Camel}}Resource manages a {{.Human}} through {{.PkgName}}.Client.{{.Service}}.
type {{.Camel}}Resource struct {
	client *{{.PkgName}}.Client
}

func new{{.Upper}}Resource() resource.Resource { return &{{.Camel}}Resource{} }

func (r *{{.Camel}}Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_{{.Name}}"
}

func (r *{{.Camel}}Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = {{.Camel}}ResourceSchema()
}

// Configure takes the client. Provider data is nil while Terraform validates
// config before the provider is configured.
func (r *{{.Camel}}Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*{{.PkgName}}.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *{{.PkgName}}.Client, got %T", req.ProviderData))
		return
	}
	r.client = client
}
{{if .Validators}}
func (r *{{.Camel}}Resource) ConfigValidators(context.Context) []resource.ConfigValidator {
	return {{.Validators}}()
}
{{end}}
func (r *{{.Camel}}Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan {{.Camel}}ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	params := plan.createParams(&resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.{{.Service}}.Create(ctx, {{.CreateArgs}}params)
	if err == nil && (out == nil || out.{{.IDField}} == "") {
		err = errNoResult
	}
	if err != nil {
		if isUnknownOutcome(err) {
			addUnknownCreateOutcome(&resp.Diagnostics, createOutcome{
				TypeName:     "honeybadger_{{.Name}}",
				ProjectID:    {{.ProjectIDExpr}},
				Name:         {{.NameExpr}},
				ImportFormat: {{.Camel}}ImportFormat,
				FindHint:     {{printf "%q" .FindHint}},
			}, err)
			return
		}
		addAPIError(ctx, &resp.Diagnostics, {{.Camel}}ResourceSchema(), "Creating {{.Human}}", err)
		return
	}
	// Start from the plan so attributes the API doesn't return, such as
	// write-only ones, keep their planned values. The framework compares the
	// result with the plan for semantic equality itself.
	state := plan
	state.fromAPI(out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *{{.Camel}}Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state {{.Camel}}ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.{{.Service}}.Get(ctx, {{.IDArgs}})
	if errors.Is(err, {{.PkgName}}.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err == nil && (out == nil || out.{{.IDField}} == "") {
		err = errNoResult
	}
	if err != nil {
		addAPIError(ctx, &resp.Diagnostics, {{.Camel}}ResourceSchema(), "Reading {{.Human}}", err)
		return
	}
	state.fromAPI(out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *{{.Camel}}Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
{{- if .UpdateParams}}
	var plan, state {{.Camel}}ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	params := plan.updateParams(state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.{{.Service}}.Update(ctx, {{.IDArgs}}, params)
	if err == nil && (out == nil || out.{{.IDField}} == "") {
		err = errNoResult
	}
	if err != nil {
		addAPIError(ctx, &resp.Diagnostics, {{.Camel}}ResourceSchema(), "Updating {{.Human}}", err)
		return
	}
	plan.fromAPI(out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
{{- else}}
	resp.Diagnostics.AddError("Unexpected update", "Every {{.Human}} attribute forces replacement, so an update shouldn't happen.")
{{- end}}
}

func (r *{{.Camel}}Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state {{.Camel}}ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.{{.Service}}.Delete(ctx, {{.IDArgs}})
	if err == nil || errors.Is(err, {{.PkgName}}.ErrNotFound) {
		return
	}
	if isUnknownOutcome(err) {
		addUnknownDeleteOutcome(&resp.Diagnostics, "honeybadger_{{.Name}}", state.ID.ValueString(), err)
		return
	}
	addAPIError(ctx, &resp.Diagnostics, {{.Camel}}ResourceSchema(), "Deleting {{.Human}}", err)
}

func (r *{{.Camel}}Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByFormat(ctx, {{.Camel}}ImportFormat, req, resp)
}
`))
