resource "honeybadger_integration_zulip" "alerts" {
  project_id = honeybadger_project.app.id

  config = {
    # One of these is required: zulip_domain for Zulip Cloud
    # (example.zulipchat.com), or server_url for a self-hosted server.
    zulip_domain = "example"

    bot_email = "honeybadger-bot@example.zulipchat.com"
    api_key   = var.zulip_api_key
    channel   = "alerts"
    topic     = "Honeybadger"
  }
}

variable "zulip_api_key" {
  type      = string
  sensitive = true
}
