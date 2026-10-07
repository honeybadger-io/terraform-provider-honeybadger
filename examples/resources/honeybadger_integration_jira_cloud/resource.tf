# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_jira_cloud" "example" {
  config = {
    project_key = "OPS"
  }
  project_id = honeybadger_project.app.id
}
