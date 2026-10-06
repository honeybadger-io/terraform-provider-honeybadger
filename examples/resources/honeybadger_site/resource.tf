resource "honeybadger_site" "api" {
  project_id = honeybadger_project.app.id
  url        = "https://api.example.com/health"
  locations  = ["Virginia", "Frankfurt"]
  match_type = "include"
  match      = "ok"

  request_headers = {
    Authorization = "Bearer ${var.health_check_token}"
  }
}
