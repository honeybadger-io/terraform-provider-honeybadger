resource "honeybadger_status_page" "public" {
  name = "Example status"

  sites = [
    { site_id = honeybadger_site.api.id, display_name = "API" },
  ]

  check_ins = [
    { check_in_id = honeybadger_check_in.nightly.id, display_name = "Nightly billing" },
  ]
}
