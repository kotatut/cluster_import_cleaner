resource "google_container_cluster" "primary" {
  name               = "my-cluster"
  location           = "us-central1"
}
