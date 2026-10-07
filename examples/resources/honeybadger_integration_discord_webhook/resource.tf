# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_discord_webhook" "example" {
  config = {
    url = var.integration_discord_webhook_config_url
  }
  project_id = honeybadger_project.app.id
}

variable "integration_discord_webhook_config_url" {
  type      = string
  sensitive = true
}
