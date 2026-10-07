# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_pager_duty_v2" "example" {
  config = {
    integration_key = var.integration_pager_duty_v2_config_integration_key
  }
  project_id = honeybadger_project.app.id
}

variable "integration_pager_duty_v2_config_integration_key" {
  type      = string
  sensitive = true
}
