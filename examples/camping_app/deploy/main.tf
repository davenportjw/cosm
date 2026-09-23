# Terraform Infrastructure Configuration
# Component: main.tf

terraform {
  required_version = ">= 1.5.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

variable "project_id" {
  type        = string
  description = "Google Cloud Project ID"
  default     = "davenport-boutique"
}

variable "region" {
  type        = string
  description = "Google Cloud Region"
  default     = "us-central1"
}

variable "image_tag" {
  type        = string
  description = "Container image URL"
  default     = "us-central1-docker.pkg.dev/davenport-boutique/cosm/camping-app:v1"
}

provider "google" {
  project = var.project_id
  region  = var.region
}

resource "google_redis_instance" "camping_redis" {
  name               = "camping-redis"
  tier               = "BASIC"
  memory_size_gb     = 1
  redis_version      = "REDIS_7_0"
  region             = var.region
  authorized_network = "default"
}

resource "google_cloud_run_v2_service" "camping_app" {
  name     = "cosm-camping-app"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    scaling {
      max_instance_count = 10
      min_instance_count = 1
    }

    containers {
      image = var.image_tag

      ports {
        container_port = 8080
      }

      resources {
        limits = {
          cpu    = "1000m"
          memory = "512Mi"
        }
      }

      env {
        name  = "PORT"
        value = "8080"
      }

      env {
        name  = "REDIS_HOST"
        value = google_redis_instance.camping_redis.host
      }

      env {
        name  = "REDIS_PORT"
        value = tostring(google_redis_instance.camping_redis.port)
      }

      env {
        name  = "GCP_PROJECT"
        value = var.project_id
      }

      env {
        name  = "VERTEX_LOCATION"
        value = "us-central1"
      }

      startup_probe {
        http_get {
          path = "/health"
          port = 8080
        }
        initial_delay_seconds = 2
        period_seconds        = 5
        failure_threshold     = 3
      }

      liveness_probe {
        http_get {
          path = "/health"
          port = 8080
        }
        period_seconds    = 15
        failure_threshold = 3
      }
    }
  }
}

resource "google_cloud_run_v2_service_iam_member" "public_access" {
  project  = google_cloud_run_v2_service.camping_app.project
  location = google_cloud_run_v2_service.camping_app.location
  name     = google_cloud_run_v2_service.camping_app.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

output "service_url" {
  description = "Deployed Cloud Run live URL"
  value       = google_cloud_run_v2_service.camping_app.uri
}
