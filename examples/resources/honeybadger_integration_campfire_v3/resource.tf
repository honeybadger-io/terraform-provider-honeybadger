# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_campfire_v3" "example" {
  config = {
    chatbot_url = "https://example.com"
  }
  project_id = honeybadger_project.app.id
}
