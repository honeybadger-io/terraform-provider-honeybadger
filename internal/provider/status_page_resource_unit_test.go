package provider

import (
	"fmt"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const statusPageAddr = "honeybadger_status_page.t"

func statusPageHCL(url, attrs string) string {
	return providerConfig(url) + fmt.Sprintf(`
resource "honeybadger_status_page" "t" {
  name = "Status"
%s}
`, attrs)
}

// Review Focus 4 and 5: the write-only password, features replaced whole,
// and a cleared domain.
func TestStatusPageWrites(t *testing.T) {
	f := newFakeHoneybadger(t)
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				Config: statusPageHCL(f.srv.URL, "  username = \"u\"\n  password = \"s1\"\n"),
				Check: tfresource.ComposeTestCheckFunc(
					lastBody(f, "POST", `/status_pages$`, map[string]any{
						"name": "Status", "username": "u", "password": "s1",
						"hide_branding": false, "password_protected": false, "message_enabled": false,
						"search_engine_indexing_disabled": false,
						"features":                        map[string]any{}, "sites": []any{}, "check_ins": []any{},
					}),
					// Never returned, the password stays in state from the plan.
					tfresource.TestCheckResourceAttr(statusPageAddr, "password", "s1"),
				),
			},
			{
				// Changed in Honeybadger, the password isn't returned, so
				// Terraform can't see the change: the plan stays empty.
				PreConfig: func() {
					f.mutate(`/status_pages/`, func(o map[string]any) { o["password"] = "changed-elsewhere" })
				},
				Config:   statusPageHCL(f.srv.URL, "  username = \"u\"\n  password = \"s1\"\n"),
				PlanOnly: true,
			},
			{
				Config: statusPageHCL(f.srv.URL, "  username = \"u\"\n"),
				Check:  lastBody(f, "PATCH", `/status_pages/`, map[string]any{"password": nil}),
			},
			{
				Config: statusPageHCL(f.srv.URL, "  username = \"u\"\n  features = { up_caption = \"Up\", down_caption = \"Down\" }\n"),
				Check:  lastBody(f, "PATCH", `/status_pages/`, map[string]any{"features": map[string]any{"up_caption": "Up", "down_caption": "Down"}}),
			},
			{
				// One key removed: the object is sent whole, without it.
				Config: statusPageHCL(f.srv.URL, "  username = \"u\"\n  features = { up_caption = \"Up\" }\n"),
				Check:  lastBody(f, "PATCH", `/status_pages/`, map[string]any{"features": map[string]any{"up_caption": "Up"}}),
			},
			{
				Config: statusPageHCL(f.srv.URL, "  username = \"u\"\n"),
				Check:  lastBody(f, "PATCH", `/status_pages/`, map[string]any{"features": map[string]any{}}),
			},
			{
				Config: statusPageHCL(f.srv.URL, "  username = \"u\"\n  domain = \"status.example.com\"\n"),
				Check:  lastBody(f, "PATCH", `/status_pages/`, map[string]any{"domain": "status.example.com"}),
			},
			{
				Config: statusPageHCL(f.srv.URL, "  username = \"u\"\n"),
				Check:  lastBody(f, "PATCH", `/status_pages/`, map[string]any{"domain": nil}),
			},
		},
	})
}

// Review Focus 3: sites are in page order, so reordering is an update; a
// null display_name reads back null.
func TestStatusPageSitesOrder(t *testing.T) {
	f := newFakeHoneybadger(t)
	a := "{ site_id = \"00000000-0000-4000-8000-000000000001\" }"
	b := "{ site_id = \"00000000-0000-4000-8000-000000000002\", display_name = \"API\" }"
	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []tfresource.TestStep{
			{
				Config: statusPageHCL(f.srv.URL, fmt.Sprintf("  sites = [%s, %s]\n", a, b)),
				Check: tfresource.ComposeTestCheckFunc(
					lastBodyKey(f, "POST", `/status_pages$`, "sites", []any{
						map[string]any{"site_id": "00000000-0000-4000-8000-000000000001"},
						map[string]any{"site_id": "00000000-0000-4000-8000-000000000002", "display_name": "API"},
					}),
					tfresource.TestCheckNoResourceAttr(statusPageAddr, "sites.0.display_name"),
					tfresource.TestCheckResourceAttr(statusPageAddr, "sites.1.display_name", "API"),
				),
			},
			{
				Config:           statusPageHCL(f.srv.URL, fmt.Sprintf("  sites = [%s, %s]\n", b, a)),
				ConfigPlanChecks: expectUpdate(statusPageAddr),
				Check: lastBody(f, "PATCH", `/status_pages/`, map[string]any{"sites": []any{
					map[string]any{"site_id": "00000000-0000-4000-8000-000000000002", "display_name": "API"},
					map[string]any{"site_id": "00000000-0000-4000-8000-000000000001"},
				}}),
			},
		},
	})
}
