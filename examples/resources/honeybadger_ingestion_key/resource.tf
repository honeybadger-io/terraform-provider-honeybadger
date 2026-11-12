resource "honeybadger_ingestion_key" "deploy" {
  project_id = honeybadger_project.app.id
  label      = "deploys"
}

# `terraform output -raw deploy_ingestion_key` prints it.
output "deploy_ingestion_key" {
  value     = honeybadger_ingestion_key.deploy.key
  sensitive = true
}
