output "service_url" {
  value = google_cloud_run_v2_service.server.uri
}

output "image_repository" {
  value = "${var.region}-docker.pkg.dev/${var.project_id}/${var.service_name}/${var.service_name}"
}
