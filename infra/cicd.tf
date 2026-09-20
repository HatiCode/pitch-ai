# Keyless deploys from GitHub Actions via Workload Identity Federation.
#
# No service-account key is ever created or stored: GitHub mints a short-lived
# OIDC token, GCP exchanges it for credentials, and the attribute condition
# below restricts that exchange to this repository alone. A long-lived key in a
# GitHub secret would be a standing credential that leaks by copy-paste.

variable "github_repository" {
  type        = string
  description = "owner/name of the repository allowed to deploy"
}

resource "google_project_service" "cicd" {
  for_each = toset([
    "iamcredentials.googleapis.com",
    "sts.googleapis.com",
  ])
  service            = each.value
  disable_on_destroy = false
}

resource "google_iam_workload_identity_pool" "github" {
  workload_identity_pool_id = "github-pool"
  display_name              = "GitHub Actions"
  depends_on                = [google_project_service.cicd]
}

resource "google_iam_workload_identity_pool_provider" "github" {
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-provider"
  display_name                       = "GitHub OIDC"

  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.repository" = "assertion.repository"
  }

  # Without this, ANY GitHub repository in the world could exchange a token for
  # these credentials. It is the single most important line in this file.
  attribute_condition = "assertion.repository == '${var.github_repository}'"

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

resource "google_service_account" "deployer" {
  account_id   = "${var.service_name}-deployer"
  display_name = "pitch-ai GitHub Actions deployer"
}

resource "google_service_account_iam_member" "deployer_wif" {
  service_account_id = google_service_account.deployer.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.repository/${var.github_repository}"
}

resource "google_project_iam_member" "deployer" {
  for_each = toset([
    "roles/run.developer",              # deploy revisions, not manage IAM
    "roles/cloudbuild.builds.editor",   # submit builds
    "roles/artifactregistry.writer",    # push the image
    "roles/storage.admin",              # Cloud Build's source staging bucket
    "roles/logging.viewer",             # read build logs
  ])
  project = var.project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.deployer.email}"
}

# Deploying a revision means acting as the runtime service account, which is a
# separate grant from being able to deploy at all.
resource "google_service_account_iam_member" "deployer_acts_as_server" {
  service_account_id = google_service_account.server.name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.deployer.email}"
}

# Builds run as this account. A build with no service account of its own falls
# back to the Compute Engine default account, which carries roles/editor: the
# build would get project-wide write access, and so would anyone able to push to
# main, since submitting a build means acting as the account it runs under.
resource "google_service_account" "build" {
  account_id   = "${var.service_name}-build"
  display_name = "pitch-ai Cloud Build worker"
}

resource "google_project_iam_member" "build" {
  for_each = toset([
    "roles/artifactregistry.writer", # push the built image
    "roles/logging.logWriter",       # required by CLOUD_LOGGING_ONLY
    "roles/storage.objectViewer",    # read the source gcloud staged
  ])
  project = var.project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.build.email}"
}

# Submitting a build is a separate grant from acting as the account it runs as.
resource "google_service_account_iam_member" "deployer_acts_as_build" {
  service_account_id = google_service_account.build.name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.deployer.email}"
}

output "wif_provider" {
  value = google_iam_workload_identity_pool_provider.github.name
}

output "deployer_service_account" {
  value = google_service_account.deployer.email
}

# Fully qualified rather than a bare email: --service-account takes the
# projects/*/serviceAccounts/* form.
output "build_service_account" {
  value = google_service_account.build.name
}
