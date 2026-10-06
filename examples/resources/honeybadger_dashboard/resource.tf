resource "honeybadger_dashboard" "ops" {
  project_id = honeybadger_project.app.id
  title      = "Operations"

  widgets = jsonencode([
    {
      id           = "error-rate"
      type         = "insights_vis"
      grid         = { x = 0, y = 0, w = 6, h = 3 }
      presentation = { title = "Errors in ${honeybadger_project.app.name}" }
      config = {
        query = "filter event_type::str == \"notice\" | stats count() as count by bin(1h) as time"
        vis   = { view = "line", chart_config = { xField = "time", yField = "count" } }
      }
    },
    {
      id     = "recent-errors"
      type   = "errors"
      grid   = { x = 6, y = 0, w = 6, h = 3 }
      config = { limit = 10, sort = "last_seen_desc" }
    },
  ])
}
