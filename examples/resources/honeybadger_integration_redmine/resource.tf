# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_redmine" "example" {
  config = {
    api_key    = var.integration_redmine_config_api_key
    server_url = "https://example.com"
  }
  project_id = honeybadger_project.app.id
}

variable "integration_redmine_config_api_key" {
  type      = string
  sensitive = true
}
