# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_victor_ops" "example" {
  config = {
    api_key = var.integration_victor_ops_config_api_key
  }
  project_id = honeybadger_project.app.id
}

variable "integration_victor_ops_config_api_key" {
  type      = string
  sensitive = true
}
