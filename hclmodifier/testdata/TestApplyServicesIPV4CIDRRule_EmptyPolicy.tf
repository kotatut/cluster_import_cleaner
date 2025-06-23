resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  ip_allocation_policy {}
}