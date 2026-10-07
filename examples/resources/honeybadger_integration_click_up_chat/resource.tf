# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_click_up_chat" "example" {
  config = {
    channel_id   = "4-90120123456-8"
    workspace_id = "9012345678"
  }
  project_id = honeybadger_project.app.id
}
