resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  ip_allocation_policy {
    cluster_ipv4_cidr_block = "10.1.0.0/14"
  }
}