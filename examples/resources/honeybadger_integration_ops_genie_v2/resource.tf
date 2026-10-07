# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_ops_genie_v2" "example" {
  config = {
    api_key = var.integration_ops_genie_v2_config_api_key
  }
  project_id = honeybadger_project.app.id
}

variable "integration_ops_genie_v2_config_api_key" {
  type      = string
  sensitive = true
}
