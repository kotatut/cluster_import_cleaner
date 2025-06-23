resource "google_container_cluster" "primary" {
  name               = "my-cluster"
  location           = "us-central1"
  initial_node_count = 3
}
