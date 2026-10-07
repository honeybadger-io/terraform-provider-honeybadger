# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_github" "example" {
  config = {
    repo = "honeybadger-io/crywolf"
  }
  project_id = honeybadger_project.app.id
}
