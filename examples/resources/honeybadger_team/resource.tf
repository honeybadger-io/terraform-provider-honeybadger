resource "honeybadger_team" "backend" {
  name        = "Backend"
  project_ids = [honeybadger_project.app.id]
}
