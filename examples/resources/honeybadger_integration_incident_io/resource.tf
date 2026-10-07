# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_incident_io" "example" {
  config = {
    alert_source_label = "Honeybadger"
    api_key            = var.integration_incident_io_config_api_key
  }
  project_id = honeybadger_project.app.id
}

variable "integration_incident_io_config_api_key" {
  type      = string
  sensitive = true
}
