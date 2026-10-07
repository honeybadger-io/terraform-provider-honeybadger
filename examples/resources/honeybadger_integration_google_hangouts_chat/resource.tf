# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_google_hangouts_chat" "example" {
  config = {
    url = var.integration_google_hangouts_chat_config_url
  }
  project_id = honeybadger_project.app.id
}

variable "integration_google_hangouts_chat_config_url" {
  type      = string
  sensitive = true
}
