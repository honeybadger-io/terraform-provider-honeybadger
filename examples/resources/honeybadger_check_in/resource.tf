resource "honeybadger_check_in" "nightly" {
  project_id    = honeybadger_project.app.id
  name          = "Nightly billing"
  report_period = "1 day"
  grace_period  = "10 minutes"
}
