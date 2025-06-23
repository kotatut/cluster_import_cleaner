resource "google_container_cluster" "primary" {
  name               = "my-cluster"
  location           = "us-central1"
  node_config {
    image_type = "COS_CONTAINERD"
  }
}
