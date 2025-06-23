resource "google_container_cluster" "primary" {
  name                   = "my-cluster"
  location               = "us-central1"
  master_ipv4_cidr_block = "10.100.0.0/28"
  private_cluster_config {
    private_endpoint_subnetwork = "my-subnetwork"
  }
}
