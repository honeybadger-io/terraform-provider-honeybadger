resource "honeybadger_check_in" "nightly" {
  project_id    = honeybadger_project.app.id
  name          = "Nightly billing"
  report_period = "1 day"
  grace_period  = "10 minutes"
}

# The job pings this URL each time it finishes, for example with
# `curl -fsS "$CHECK_IN_URL"` at the end of the cron script.
output "nightly_check_in_url" {
  value = honeybadger_check_in.nightly.report_url
}
