# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_intercom" "example" {
  config = {
    context_field = "user_id"
  }
  project_id = honeybadger_project.app.id
}
