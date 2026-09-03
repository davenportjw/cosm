output "cloud_run_url" {
  description = "The deployed Google Cloud Run Web UI URL."
  value       = google_cloud_run_v2_service.agent_service.uri
}

output "cloud_sql_connection_name" {
  description = "The Cloud SQL instance connection string for Cloud Run volume mounts."
  value       = google_sql_database_instance.postgres.connection_name
}

output "cloud_sql_public_ip" {
  description = "The public IP address of the Cloud SQL instance."
  value       = google_sql_database_instance.postgres.public_ip_address
}

output "service_account_email" {
  description = "The service account email attached to Cloud Run."
  value       = google_service_account.agent_sa.email
}
