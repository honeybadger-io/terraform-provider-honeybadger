# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_click_up" "example" {
  config = {
    initial_status_id = "sc901201234567_AbCd1234"
    list_id           = "901201234567"
    reopen_status_id  = "sc901201234567_AbCd1234"
    resolve_status_id = "sc901201234567_EfGh5678"
    teamspace_id      = "90120123456"
    workspace_id      = "9012345678"
  }
  project_id = honeybadger_project.app.id
}
