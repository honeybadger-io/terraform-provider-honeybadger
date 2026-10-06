resource "honeybadger_environment" "staging" {
  project_id    = honeybadger_project.app.id
  name          = "staging"
  notifications = false
}
