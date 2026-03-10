resource "google_project_iam_member" "contract_tasker_cb_deploy_log_writer" {
  project = "peak-key-458413-b2"
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:contract-tasker-cb-deploy@peak-key-458413-b2.iam.gserviceaccount.com"
}
