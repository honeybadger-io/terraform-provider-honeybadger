# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_trello" "example" {
  config = {
    board_id = "5f1a2b3c4d5e6f7a8b9c0d1e"
    list_id  = "5f1a2b3c4d5e6f7a8b9c0d2f"
  }
  project_id = honeybadger_project.app.id
}
