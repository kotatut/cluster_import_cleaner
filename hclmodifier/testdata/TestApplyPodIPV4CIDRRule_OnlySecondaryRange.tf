resource "google_container_cluster" "primary" {
  name               = "my-cluster"
  location           = "us-central1"
  ip_allocation_policy {
    cluster_secondary_range_name = "my-pod-range"
  }
}
