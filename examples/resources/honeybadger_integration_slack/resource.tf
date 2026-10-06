resource "honeybadger_integration_slack" "team" {
  project_id = honeybadger_project.app.id
  events     = ["occurred", "resolved"]
}

# A Slack integration sends nothing until someone connects it to a Slack
# workspace. Open this link in Honeybadger to do that.
output "slack_connect_url" {
  value = honeybadger_integration_slack.team.links.web
}
