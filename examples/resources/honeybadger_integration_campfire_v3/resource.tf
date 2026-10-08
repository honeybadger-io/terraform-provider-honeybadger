# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_campfire_v3" "example" {
  config = {
    chatbot_url = var.integration_campfire_v3_config_chatbot_url
  }
  project_id = honeybadger_project.app.id
}

variable "integration_campfire_v3_config_chatbot_url" {
  type      = string
  sensitive = true
}
