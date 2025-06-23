resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  cluster_ipv4_cidr  = "10.0.0.0/14"
  ip_allocation_policy {
    services_ipv4_cidr_block = "10.2.0.0/20"
  }
}