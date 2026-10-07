# A minimal configuration, generated from the API spec's examples.
resource "honeybadger_integration_backlog" "example" {
  config = {
    api_key            = var.integration_backlog_config_api_key
    backlog_project_id = "123456"
    issue_type_id      = "1074711"
    priority_id        = "3"
    reopen_status_id   = "1"
    resolve_status_id  = "4"
    space_url          = "https://example.backlog.com"
  }
  project_id = honeybadger_project.app.id
}

variable "integration_backlog_config_api_key" {
  type      = string
  sensitive = true
}
