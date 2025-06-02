# Scenario 1: Rule 3 should trigger - binary_authorization with enabled and evaluation_mode
resource "google_container_cluster" "gke_test_rule3_conflict" {
  name     = "rule3-conflict-cluster"
  location = "us-central1-a"
  binary_authorization {
    evaluation_mode = "PROJECT_SINGLETON_POLICY_ENFORCE"
  }
  ip_allocation_policy {
    cluster_ipv4_cidr_block  = "10.1.0.0/16"
    services_ipv4_cidr_block = "10.2.0.0/20" # Rule 2 might also trigger here if not careful
  }
}

# Scenario 2: Rule 3 should NOT trigger - only evaluation_mode
resource "google_container_cluster" "gke_test_rule3_no_conflict_eval_mode" {
  name     = "rule3-no-conflict-eval-cluster"
  location = "us-central1-a"
  binary_authorization {
    evaluation_mode = "DISABLED"
  }
}

# Scenario 3: Rule 3 should NOT trigger - only enabled (deprecated but no conflict for Rule 3)
resource "google_container_cluster" "gke_test_rule3_no_conflict_enabled_only" {
  name     = "rule3-no-conflict-enabled-cluster"
  location = "us-central1-a"
  binary_authorization {
    enabled = true
  }
}

# Scenario 4: Rule 3 should NOT trigger - binary_authorization block missing
resource "google_container_cluster" "gke_test_rule3_no_block" {
  name     = "rule3-no-block-cluster"
  location = "us-central1-a"
}

# Scenario 5: Rule 1 trigger
resource "google_container_cluster" "gke_test_rule1_conflict" {
  name     = "rule1-conflict-cluster"
  location = "us-central1-a"
  ip_allocation_policy {
    cluster_ipv4_cidr_block = "10.1.0.0/14"
  }
}

# Scenario 6: Rule 2 trigger
resource "google_container_cluster" "gke_test_rule2_conflict" {
  name     = "rule2-conflict-cluster"
  location = "us-central1-a"
  ip_allocation_policy {
    cluster_secondary_range_name = "services_range"
  }
}

# Scenario 7: Non-GKE resource, should be untouched
resource "google_compute_instance" "test_vm" {
  name         = "test-vm"
  machine_type = "e2-medium"
  zone         = "us-central1-a"
  binary_authorization { # This should NOT be processed by Rule 3
    enabled         = true
    evaluation_mode = "PROJECT_SINGLETON_POLICY_ENFORCE"
  }
}
