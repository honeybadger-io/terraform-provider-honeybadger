package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

var testFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"honeybadger": providerserver.NewProtocol6WithError(New("test")()),
}

func providerConfig(url string) string {
	return fmt.Sprintf("provider \"honeybadger\" {\n  api_token = \"hbt_test\"\n  api_url   = %q\n}\n", url)
}

type fakeRequest struct {
	Method, Path string
	Body         map[string]any
}

type fakeResponse struct {
	Status  int
	Body    string
	Headers map[string]string
}

// fakeHoneybadger is an in-memory v3 API for the project, project key,
// check-in, alarm, integration, environment, site, team and status page
// endpoints. It records every request and fills server-side defaults
// the way the API does. It isn't a general emulator (spec 8.2).
type fakeHoneybadger struct {
	srv      *httptest.Server
	mu       sync.Mutex
	objects  map[string]map[string]any // object path -> object
	requests []fakeRequest
	scripted map[string][]fakeResponse // "METHOD path" (or "METHOD *") -> queued responses
	next     int
	// projectStreams are the streams a project has; nil means both.
	projectStreams []string
}

var (
	collectionPath = regexp.MustCompile(`^/v3/(?:projects/([^/]+)/(check_ins|keys|alarms|integrations|environments|sites|dashboards)|(projects|teams|status_pages))$`)
	objectPath     = regexp.MustCompile(`^/v3/(?:projects/[^/]+/(?:check_ins|keys|alarms|integrations|environments|sites|dashboards)|projects|teams|status_pages)/[^/]+$`)
)

func newFakeHoneybadger(t *testing.T) *fakeHoneybadger {
	t.Helper()
	f := &fakeHoneybadger{objects: map[string]map[string]any{}, scripted: map[string][]fakeResponse{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

// script queues a response for the next request matching method and path.
// A path of "*" matches any path.
func (f *fakeHoneybadger) script(method, path string, r fakeResponse) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := method + " " + path
	f.scripted[k] = append(f.scripted[k], r)
}

// bodies returns the decoded bodies of the requests with this method whose
// path matches pattern.
func (f *fakeHoneybadger) bodies(method, pattern string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	re := regexp.MustCompile(pattern)
	var out []map[string]any
	for _, r := range f.requests {
		if r.Method == method && re.MatchString(r.Path) {
			out = append(out, r.Body)
		}
	}
	return out
}

// remove deletes every stored object whose path matches pattern, as if it
// were deleted outside Terraform.
func (f *fakeHoneybadger) remove(pattern string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	re := regexp.MustCompile(pattern)
	for p := range f.objects {
		if re.MatchString(p) {
			delete(f.objects, p)
		}
	}
}

// snapshot is an object as a scripted response body would carry it.
func (f *fakeHoneybadger) snapshot(pattern string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	re := regexp.MustCompile(pattern)
	for p, obj := range f.objects {
		if re.MatchString(p) {
			b, _ := json.Marshal(map[string]any{"data": obj})
			return string(b)
		}
	}
	return ""
}

func (f *fakeHoneybadger) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var body map[string]any
	if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}
	f.requests = append(f.requests, fakeRequest{Method: r.Method, Path: r.URL.Path, Body: body})

	for _, k := range []string{r.Method + " " + r.URL.Path, r.Method + " *"} {
		if q := f.scripted[k]; len(q) > 0 {
			f.scripted[k] = q[1:]
			for h, v := range q[0].Headers {
				w.Header().Set(h, v)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(q[0].Status)
			_, _ = io.WriteString(w, q[0].Body)
			return
		}
	}

	if r.Method == http.MethodPost {
		if m := collectionPath.FindStringSubmatch(r.URL.Path); m != nil {
			kind := m[2]
			if kind == "" {
				kind = m[3]
			}
			f.create(w, r.URL.Path, m[1], kind, body)
			return
		}
	}
	if objectPath.MatchString(r.URL.Path) {
		obj, ok := f.objects[r.URL.Path]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"code": "not_found", "message": "Not found"}})
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]any{"data": readable(obj)})
		case http.MethodPatch:
			if msg := badStream(body); msg != "" {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]any{"code": "invalid", "message": msg}})
				return
			}
			for k, v := range body {
				if k == "config" {
					// v3 merges config: a key left out keeps its value; null deletes it.
					stored, _ := obj["config"].(map[string]any)
					for ck, cv := range v.(map[string]any) {
						if cv == nil {
							delete(stored, ck)
						} else {
							stored[ck] = cv
						}
					}
					continue
				}
				obj[k] = v
			}
			if ws, ok := obj["widgets"].([]any); ok {
				obj["widgets"] = f.storedWidgets(ws)
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": readable(obj)})
		case http.MethodDelete:
			delete(f.objects, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		}
		return
	}
	http.NotFound(w, r)
}

func (f *fakeHoneybadger) create(w http.ResponseWriter, collection, projectID, kind string, body map[string]any) {
	if msg := badStream(body); msg != "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]any{"code": "invalid", "message": msg}})
		return
	}
	f.next++
	id := fmt.Sprintf("x%d", f.next)
	if kind == "sites" {
		id = fmt.Sprintf("00000000-0000-4000-8000-%012d", f.next) // site IDs are UUIDs
	}
	obj := map[string]any{"id": id, "created_at": "2026-10-05T00:00:00Z"}
	switch kind {
	case "projects":
		obj["account_id"] = "a1"
		obj["disable_public_links"] = false
		obj["resolve_errors_on_deploy"] = true
	case "check_ins":
		obj["project_id"] = projectID
		obj["schedule_type"] = "simple"
		obj["grace_period"] = "00:00:00"
		obj["cron_timezone"] = "UTC"
		obj["report_url"] = "https://example.com/check_in/" + id
	case "keys":
		obj["project_id"] = projectID
		obj["key"] = "hbp_" + id
	case "alarms":
		obj["project_id"] = projectID
		obj["state"] = "initial"
		obj["updated_at"] = "2026-10-05T00:00:00Z"
		obj["stream_ids"] = []any{"s-default", "s-internal"}
	case "integrations":
		obj["project_id"] = projectID
		obj["links"] = map[string]any{"web": "https://app.honeybadger.io/projects/" + projectID + "/integrations/" + id}
		obj["events"] = []any{"occurred"}
		// site_ids and check_in_ids left out follow every site and check-in,
		// which reads as null (honeybadger 854fd20da7).
		obj["site_ids"], obj["check_in_ids"] = nil, nil
		for _, k := range []string{"alarm_alert_ids", "alarm_ok_ids", "included_environments", "excluded_environments", "filters"} {
			obj[k] = []any{}
		}
		obj["rate"], obj["threshold"] = "wday", 100
		stored := map[string]any{}
		if cfg, ok := body["config"].(map[string]any); ok {
			maps.Copy(stored, cfg)
		}
		obj["config"] = stored
	case "environments":
		obj["project_id"] = projectID
		obj["notifications"] = true
	case "sites":
		obj["project_id"] = projectID
		obj["name"] = body["url"] // name defaults to the URL
		obj["links"] = map[string]any{"web": "https://app.honeybadger.io/projects/" + projectID + "/sites/" + id}
		obj["state"], obj["last_checked_at"] = "pending", nil
		obj["frequency"], obj["match_type"], obj["match"], obj["request_method"] = 5, "success", nil, "GET"
		obj["validate_ssl"], obj["active"] = true, true
		obj["locations"], obj["request_headers"] = []any{}, map[string]any{}
		obj["request_body"], obj["outage_threshold"], obj["timeout"] = nil, nil, nil
	case "dashboards":
		obj["project_id"] = projectID
		obj["is_default"], obj["shared"], obj["default_ts"] = false, false, nil
		obj["updated_at"] = "2026-10-05T00:00:00Z"
		obj["links"] = map[string]any{"web": "https://app.honeybadger.io/projects/" + projectID + "/insights/dashboards/" + id}
		obj["widgets"] = []any{}
	case "teams":
		obj["account_id"], obj["member_count"] = "a1", 0
		obj["project_ids"] = []any{}
		obj["links"] = map[string]any{"web": "https://app.honeybadger.io/teams/" + id}
	case "status_pages":
		obj["account_id"] = "a1"
		obj["url"] = "https://status.example.com/" + id
		obj["links"] = map[string]any{"web": "https://app.honeybadger.io/status_pages/" + id}
		for _, k := range []string{"domain", "domain_verified_at", "username", "message_title", "message_body", "google_analytics_id"} {
			obj[k] = nil
		}
		for _, k := range []string{"hide_branding", "password_protected", "incidents_enabled", "search_engine_indexing_disabled", "message_enabled"} {
			obj[k] = false
		}
		obj["features"], obj["sites"], obj["check_ins"] = map[string]any{}, []any{}, []any{}
	}
	for k, v := range body {
		if k == "config" {
			continue
		}
		obj[k] = v
	}
	if ws, ok := obj["widgets"].([]any); ok {
		obj["widgets"] = f.storedWidgets(ws)
	}
	f.objects[collection+"/"+id] = obj
	writeJSON(w, http.StatusCreated, map[string]any{"data": readable(obj)})
}

// writeOnlyConfigKeys are the write-only integration settings of the types
// the tests use (WebHook, Jira); v3 never returns them.
var writeOnlyConfigKeys = map[string]bool{"bearer_token": true, "password": true}

// readable is an object as the API returns it: without write-only keys (a
// status page's password, integration config secrets).
func readable(obj map[string]any) map[string]any {
	out := maps.Clone(obj)
	delete(out, "password")
	if cfg, ok := obj["config"].(map[string]any); ok {
		read := map[string]any{}
		for k, v := range cfg {
			if !writeOnlyConfigKeys[k] {
				read[k] = v
			}
		}
		out["config"] = read
	}
	return out
}

// mutate changes a stored object, as an edit in Honeybadger's UI would.
func (f *fakeHoneybadger) mutate(pattern string, change func(map[string]any)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	re := regexp.MustCompile(pattern)
	for p, obj := range f.objects {
		if re.MatchString(p) {
			change(obj)
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// storedWidgets is what Honeybadger stores and reads back for dashboard
// widgets (honeybadger a15c6fbb39): an id for one left without, the grid
// merged into its default, every stream for an insights_vis widget whose
// streams are left out or empty, and null, "", [] and {} dropped.
func (f *fakeHoneybadger) storedWidgets(in []any) []any {
	have := map[string]bool{"default": true, "internal": true}
	if f.projectStreams != nil {
		have = map[string]bool{}
		for _, s := range f.projectStreams {
			have[s] = true
		}
	}
	out := make([]any, 0, len(in))
	for i, e := range in {
		w, _ := e.(map[string]any)
		w = maps.Clone(w)
		if w == nil {
			w = map[string]any{}
		}
		if s, _ := w["id"].(string); s == "" {
			w["id"] = fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1)
		}
		grid := map[string]any{"x": float64(0), "y": float64(0), "w": float64(12), "h": float64(3)}
		if g, ok := w["grid"].(map[string]any); ok {
			maps.Copy(grid, g)
		}
		w["grid"] = grid
		if w["type"] == "insights_vis" {
			cfg, _ := w["config"].(map[string]any)
			cfg = maps.Clone(cfg)
			if cfg == nil {
				cfg = map[string]any{}
			}
			if s, _ := cfg["streams"].([]any); len(s) == 0 {
				var all []any
				for _, st := range []string{"default", "internal"} {
					if have[st] {
						all = append(all, st)
					}
				}
				cfg["streams"] = all
			} else {
				// A valid slug the project has no stream for yet is dropped.
				var kept []any
				for _, st := range s {
					if name, _ := st.(string); have[name] {
						kept = append(kept, st)
					}
				}
				cfg["streams"] = kept
			}
			w["config"] = cfg
		}
		out = append(out, compact(w))
	}
	return out
}

// badStream is the 422 Honeybadger answers for a stream slug outside
// [default, internal]; "" when every slug is valid.
func badStream(body map[string]any) string {
	ws, _ := body["widgets"].([]any)
	for i, e := range ws {
		w, _ := e.(map[string]any)
		cfg, _ := w["config"].(map[string]any)
		ss, _ := cfg["streams"].([]any)
		for _, st := range ss {
			if st != "default" && st != "internal" {
				return fmt.Sprintf("widgets[%d].config.streams: %v isn't one of default, internal", i, st)
			}
		}
	}
	return ""
}

// compact drops null, "", [] and {} at every level, as Honeybadger's
// deep_compact does.
func compact(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if c := compact(e); !emptyJSON(c) {
				out[k] = c
			}
		}
		return out
	case []any:
		out := []any{}
		for _, e := range x {
			if c := compact(e); !emptyJSON(c) {
				out = append(out, c)
			}
		}
		return out
	}
	return v
}

func emptyJSON(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}
