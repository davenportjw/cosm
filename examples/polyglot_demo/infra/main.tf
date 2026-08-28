terraform {
  required_version = ">= 1.0.0"
}

variable "project_id" {
  type        = string
  description = "GCP Project ID"
  default     = "future-of-git-demo"
}

resource "google_cloud_run_v2_service" "api_service" {
  name     = "fg-api-service"
  location = "us-central1"

  template {
    containers {
      image = "gcr.io/future-of-git-demo/api:latest"
      env {
        name  = "PORT"
        value = "8080"
      }
    }
  }
}

output "service_uri" {
  value       = google_cloud_run_v2_service.api_service.uri
  description = "Public URI for Cloud Run API Service"
}
