# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_email" "example" {
  config = {
    email = "alerts@example.com"
  }
  project_id = honeybadger_project.app.id
}
