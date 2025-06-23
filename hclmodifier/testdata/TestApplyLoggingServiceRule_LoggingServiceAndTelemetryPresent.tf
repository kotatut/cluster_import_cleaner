resource "google_container_cluster" "primary" {
  name             = "primary-cluster"
  logging_service = "logging.googleapis.com/kubernetes"
  cluster_telemetry {
    type = "ENABLED"
  }
}