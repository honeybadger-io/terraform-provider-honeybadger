package provider

import (
	"encoding/json"
	"fmt"
	"io"
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

// fakeHoneybadger is an in-memory v3 API for the project, project key and
// check-in endpoints. It records every request and fills server-side defaults
// the way the API does. It isn't a general emulator (spec 8.2).
type fakeHoneybadger struct {
	srv      *httptest.Server
	mu       sync.Mutex
	objects  map[string]map[string]any // object path -> object
	requests []fakeRequest
	scripted map[string][]fakeResponse // "METHOD path" (or "METHOD *") -> queued responses
	next     int
}

var (
	collectionPath = regexp.MustCompile(`^/v3/projects(?:/([^/]+)/(check_ins|keys))?$`)
	objectPath     = regexp.MustCompile(`^/v3/projects/([^/]+)(?:/(check_ins|keys)/([^/]+))?$`)
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
			f.create(w, r.URL.Path, m[1], m[2], body)
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
			writeJSON(w, http.StatusOK, map[string]any{"data": obj})
		case http.MethodPatch:
			for k, v := range body {
				obj[k] = v
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": obj})
		case http.MethodDelete:
			delete(f.objects, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		}
		return
	}
	http.NotFound(w, r)
}

func (f *fakeHoneybadger) create(w http.ResponseWriter, collection, projectID, kind string, body map[string]any) {
	f.next++
	id := fmt.Sprintf("x%d", f.next)
	obj := map[string]any{"id": id, "created_at": "2026-10-05T00:00:00Z"}
	switch kind {
	case "":
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
	}
	for k, v := range body {
		obj[k] = v
	}
	f.objects[collection+"/"+id] = obj
	writeJSON(w, http.StatusCreated, map[string]any{"data": obj})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
