# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_alert_ops" "example" {
  config = {
    url = var.integration_alert_ops_config_url
  }
  project_id = honeybadger_project.app.id
}

variable "integration_alert_ops_config_url" {
  type      = string
  sensitive = true
}
