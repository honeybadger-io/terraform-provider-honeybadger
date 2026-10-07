# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_datadog" "example" {
  config = {
    api_key = var.integration_datadog_config_api_key
  }
  project_id = honeybadger_project.app.id
}

variable "integration_datadog_config_api_key" {
  type      = string
  sensitive = true
}
