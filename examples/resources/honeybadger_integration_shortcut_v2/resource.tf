# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_shortcut_v2" "example" {
  config = {
    api_key            = var.integration_shortcut_v2_config_api_key
    initial_state      = "500000007"
    organization_name  = "yourorg"
    reopen_transition  = "500000007"
    resolve_transition = "500000010"
    team               = "5f1a2b3c-4d5e-6f7a-8b9c-0d1e2f3a4b5c"
  }
  project_id = honeybadger_project.app.id
}

variable "integration_shortcut_v2_config_api_key" {
  type      = string
  sensitive = true
}
