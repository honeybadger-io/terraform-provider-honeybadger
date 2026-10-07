# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_microsoft_teams_v2" "example" {
  config = {
    url = var.integration_microsoft_teams_v2_config_url
  }
  project_id = honeybadger_project.app.id
}

variable "integration_microsoft_teams_v2_config_url" {
  type      = string
  sensitive = true
}
