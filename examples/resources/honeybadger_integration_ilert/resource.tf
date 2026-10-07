# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_ilert" "example" {
  config = {
    url = var.integration_ilert_config_url
  }
  project_id = honeybadger_project.app.id
}

variable "integration_ilert_config_url" {
  type      = string
  sensitive = true
}
