variable "project_id" {
  description = "The Google Cloud Platform Project ID."
  type        = string
  default     = "my-gcp-project-id"
}

variable "region" {
  description = "The Google Cloud region for Cloud Run and Cloud SQL."
  type        = string
  default     = "us-central1"
}

variable "service_name" {
  description = "Name of the Google Cloud Run service."
  type        = string
  default     = "adk-agent-service"
}

variable "db_instance_name" {
  description = "Name of the Google Cloud SQL PostgreSQL instance."
  type        = string
  default     = "adk-postgres-instance"
}

variable "db_tier" {
  description = "Machine type for the Cloud SQL PostgreSQL instance."
  type        = string
  default     = "db-f1-micro"
}

variable "db_name" {
  description = "Database name for the ADK agent."
  type        = string
  default     = "adk_agent_db"
}

variable "db_user" {
  description = "Database user for the ADK agent."
  type        = string
  default     = "adk_agent_user"
}

variable "db_password" {
  description = "Database password for the ADK agent user."
  type        = string
  sensitive   = true
  default     = "change-me-to-a-secure-password"
}

variable "container_image" {
  description = "Container image URL to deploy to Cloud Run."
  type        = string
  default     = "gcr.io/my-gcp-project-id/adk-agent-service:latest"
}
