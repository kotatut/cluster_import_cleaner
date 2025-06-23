resource "google_container_cluster" "primary" {
  name                 = "my-cluster"
  location             = "us-central1"
  monitoring_service   = "monitoring.googleapis.com/kubernetes"
}
