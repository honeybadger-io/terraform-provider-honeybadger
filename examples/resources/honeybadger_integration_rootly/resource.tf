# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_rootly" "example" {
  config = {
    webhook_authorization_token = var.integration_rootly_config_webhook_authorization_token
  }
  project_id = honeybadger_project.app.id
}

variable "integration_rootly_config_webhook_authorization_token" {
  type      = string
  sensitive = true
}
