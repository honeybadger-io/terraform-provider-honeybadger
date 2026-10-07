# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_asana" "example" {
  config = {
    asana_project_id = "3456789012345"
  }
  project_id = honeybadger_project.app.id
}
