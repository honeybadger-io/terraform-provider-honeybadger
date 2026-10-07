# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_sprintly" "example" {
  config = {
    account_email = "dev@example.com"
    api_key       = var.integration_sprintly_config_api_key
    product_id    = "1337"
  }
  project_id = honeybadger_project.app.id
}

variable "integration_sprintly_config_api_key" {
  type      = string
  sensitive = true
}
