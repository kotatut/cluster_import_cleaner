resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  node_version       = "1.22.8-gke.200"
  min_master_version = "1.22.8-gke.200"
}