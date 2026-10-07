# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_linear" "example" {
  config = {
    team_id = "9cfb482a-81e3-4154-b5b9-2c805e70a02d"
  }
  project_id = honeybadger_project.app.id
}
