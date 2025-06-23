resource "google_container_cluster" "primary" {
  name               = "my-cluster"
  location           = "us-central1"
  cluster_autoscaling {
    auto_provisioning_defaults {
      disk_size_gb = 100
    }
  }
}
