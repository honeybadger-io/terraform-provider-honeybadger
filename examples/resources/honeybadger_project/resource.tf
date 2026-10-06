resource "honeybadger_project" "app" {
  name     = "My App"
  language = "ruby"
}

# Other resources take the project's ID, as honeybadger_project.app.id.
output "project_id" {
  value = honeybadger_project.app.id
}
