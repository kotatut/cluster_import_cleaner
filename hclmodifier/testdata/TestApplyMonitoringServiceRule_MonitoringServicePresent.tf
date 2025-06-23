resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  monitoring_service = "monitoring.googleapis.com/kubernetes"
}