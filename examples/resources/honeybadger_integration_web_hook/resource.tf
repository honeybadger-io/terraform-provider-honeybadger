resource "honeybadger_integration_web_hook" "alerts" {
  project_id = honeybadger_project.app.id
  events     = ["occurred", "resolved"]

  config = {
    url          = "https://example.com/honeybadger"
    bearer_token = var.webhook_token
  }
}

variable "webhook_token" {
  type      = string
  sensitive = true
}
