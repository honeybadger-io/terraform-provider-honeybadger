resource "honeybadger_ingestion_key" "deploy" {
  project_id = honeybadger_project.app.id
  label      = "deploys"
}

# The app sends data with this Ingestion Key: set it as `ingestion_key` in the
# client's config (`api_key` in older clients). It isn't a secret: it ships in
# the app, so it can go into the app's config as is.
output "deploy_ingestion_key" {
  value = honeybadger_ingestion_key.deploy.key
}
