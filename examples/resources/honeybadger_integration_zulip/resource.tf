# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_zulip" "example" {
  config = {
    api_key   = var.integration_zulip_config_api_key
    bot_email = "honeybadger-bot@example.zulipchat.com"
    channel   = "alerts"
    topic     = "Honeybadger"
  }
  project_id = honeybadger_project.app.id
}

variable "integration_zulip_config_api_key" {
  type      = string
  sensitive = true
}
