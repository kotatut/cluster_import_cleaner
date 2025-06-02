resource "google_container_cluster" "cluster_1_rule1_triggered" {
  name     = "cluster-1"
  location = "us-central1"

  ip_allocation_policy {
    cluster_ipv4_cidr_block  = "10.114.0.0/14"
    services_ipv4_cidr_block = "10.97.0.0/20" // Should NOT be removed by Rule 2 (no cluster_secondary_range_name)
  }
}

resource "google_container_cluster" "cluster_2_rule2_triggered" {
  name     = "cluster-2"
  location = "us-central1"
  // No cluster_ipv4_cidr, Rule 1 not triggered

  ip_allocation_policy {
    cluster_ipv4_cidr_block       = "10.115.0.0/14"
    cluster_secondary_range_name  = "pods"
    services_secondary_range_name = "services"
  }
}

resource "google_container_cluster" "cluster_3_both_rules_triggered" {
  name     = "cluster-3"
  location = "us-east1"

  ip_allocation_policy {
    cluster_ipv4_cidr_block      = "10.116.0.0/14"
    cluster_secondary_range_name = "my-pods-range"
  }
}

resource "google_container_cluster" "cluster_4_no_rules_triggered" {
  name     = "cluster-4"
  location = "us-west1"
  // No cluster_ipv4_cidr

  ip_allocation_policy {
    // No cluster_ipv4_cidr_block
    // No services_ipv4_cidr_block
    // No cluster_secondary_range_name
    services_secondary_range_name = "my-services-range"
  }
}

resource "google_container_cluster" "cluster_5_rule1_no_ip_alloc" {
  name              = "cluster-5"
  location          = "europe-west1"
  cluster_ipv4_cidr = "10.30.0.0/14" // Should NOT be removed (no ip_allocation_policy.cluster_ipv4_cidr_block)
}

resource "google_compute_instance" "random_instance" {
  name         = "test-vm"
  machine_type = "e2-medium"
  // This resource should be ignored by both rules.
  ip_allocation_policy { // Adding a similar structure to ensure it's not picked up
    cluster_ipv4_cidr_block      = "1.2.3.4/16"
    services_ipv4_cidr_block     = "5.6.7.8/20"
    cluster_secondary_range_name = "false-positive"
  }
}
