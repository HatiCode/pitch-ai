variable "project_id" {
  type        = string
  description = "GCP project ID"
}

variable "region" {
  type        = string
  description = "Region for Cloud Run and Artifact Registry"
  default     = "europe-west1"
}

variable "service_name" {
  type        = string
  description = "Cloud Run service name"
  default     = "pitch-ai"
}
