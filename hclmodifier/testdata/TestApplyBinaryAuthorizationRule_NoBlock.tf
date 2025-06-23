resource "google_container_cluster" "primary" {
  name     = "primary-cluster"
  location = "us-central1"
}