resource "honeybadger_status_page" "public" {
  name = "Example status"

  sites = [
    { site_id = honeybadger_site.api.id, display_name = "API" },
  ]

  check_ins = [
    { check_in_id = honeybadger_check_in.nightly.id, display_name = "Nightly billing" },
  ]
}

# The page's public address, to link from your site or docs.
output "status_page_url" {
  value = honeybadger_status_page.public.url
}

# Where to edit the page in Honeybadger.
output "status_page_admin_url" {
  value = honeybadger_status_page.public.links.web
}
