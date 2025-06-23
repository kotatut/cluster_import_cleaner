resource "google_container_cluster" "primary" {
  name               = "my-cluster"
  location           = "us-central1"
  enable_autopilot   = true
  node_pools {
    name = "default-node-pool"
  }
}
