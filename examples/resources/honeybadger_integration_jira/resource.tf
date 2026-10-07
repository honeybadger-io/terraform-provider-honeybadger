# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_jira" "example" {
  config = {
    password    = var.integration_jira_config_password
    project_key = "OPS"
    server_url  = "https://jira.example.com"
    username    = "ops@example.com"
  }
  project_id = honeybadger_project.app.id
}

variable "integration_jira_config_password" {
  type      = string
  sensitive = true
}
