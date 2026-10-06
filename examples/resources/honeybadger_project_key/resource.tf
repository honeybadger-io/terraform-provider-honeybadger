resource "honeybadger_project_key" "deploy" {
  project_id = honeybadger_project.app.id
  label      = "deploys"
}

# The app reports errors with this Project Key. It isn't a secret: it ships
# in the app, so it can go into the app's config as is.
output "deploy_project_key" {
  value = honeybadger_project_key.deploy.key
}
