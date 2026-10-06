resource "honeybadger_alarm" "errors" {
  project_id        = honeybadger_project.app.id
  name              = "Error spike"
  query             = "filter event_type::str == \"notice\" | stats count() as count"
  evaluation_period = "10m"
  lookback_lag      = "1m"

  trigger_config = {
    type = "alert_result_count"
    config = {
      operator = "gt"
      value    = 100
    }
  }
}
