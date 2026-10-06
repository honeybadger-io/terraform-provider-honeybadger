resource "honeybadger_project_key" "deploy" {
  project_id = honeybadger_project.app.id
  label      = "deploys"
}
