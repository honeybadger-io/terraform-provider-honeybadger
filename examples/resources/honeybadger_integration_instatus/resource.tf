# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_instatus" "example" {
  config = {
    url = var.integration_instatus_config_url
  }
  project_id = honeybadger_project.app.id
}

variable "integration_instatus_config_url" {
  type      = string
  sensitive = true
}
