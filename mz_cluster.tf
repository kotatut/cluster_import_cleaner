provider "google" {
	project = "steel-thread-clusters"
}

resource "google_container_cluster" "multizone_cluster" {
  name       = "multizone-cluster"
  location   = "us-west1"
  network    = "st-network"
  subnetwork = "st-network"

  # We define all node pools separately, so we remove the default one.
  remove_default_node_pool = true
  initial_node_count       = 1

  # --- Release Channel & Versioning ---
  # Using a release channel is best practice.
  # The version will be managed by the channel.
  release_channel {
    channel = "REGULAR"
  }

  # --- Addons & Feature Configurations ---
  addons_config {
    http_load_balancing {
      disabled = false
    }
    horizontal_pod_autoscaling {
      disabled = false
    }
    gce_persistent_disk_csi_driver_config {
      enabled = true
    }
    dns_cache_config {
      enabled = true
    }
  }

  # --- Security Configurations ---
  binary_authorization {
    evaluation_mode = "PROJECT_SINGLETON_POLICY_ENFORCE"
  }

  database_encryption {
    state = "DECRYPTED"
  }

  enable_shielded_nodes = true

  workload_identity_config {
    workload_pool = "steel-thread-clusters.svc.id.goog"
  }

  # --- Networking ---
  # Using GKE Dataplane V2, which includes network policy enforcement.
  # The conflicting "network_policy" block for Calico has been removed.
  datapath_provider           = "ADVANCED_DATAPATH"
  enable_intranode_visibility = true

  ip_allocation_policy {
    cluster_secondary_range_name  = "naame2"
    services_secondary_range_name = "naame"
  }

  # --- Logging & Monitoring ---
  logging_config {
    enable_components = ["SYSTEM_COMPONENTS", "WORKLOADS", "APISERVER", "CONTROLLER_MANAGER", "SCHEDULER"]
  }

  monitoring_config {
    # CORRECTED: Removed invalid enums like KUBELET and APISERVER to prevent API errors.
    enable_components = ["SYSTEM_COMPONENTS"]
    managed_prometheus {
      enabled = true
    }
  }

  # --- Other Cluster-wide Settings ---
  vertical_pod_autoscaling {
    enabled = true
  }
  default_max_pods_per_node = 110
}

# --- Node Pool Definitions ---

resource "google_container_node_pool" "primary_pool" {
  name     = "primary-pool"
  cluster  = google_container_cluster.multizone_cluster.name
  location = google_container_cluster.multizone_cluster.location

  # For regional clusters, specify the zones for the node pool
  node_locations = ["us-west1-a", "us-west1-b", "us-west1-c"]

  # Autoscaling is enabled, so we don't set a static node_count.
  autoscaling {
    min_node_count   = 1
    max_node_count   = 5
    location_policy  = "BALANCED"
  }

  management {
    auto_repair  = true
    auto_upgrade = true
  }

  max_pods_per_node = 110

  node_config {
    machine_type    = "e2-standard-4"
    disk_size_gb    = 100
    disk_type       = "pd-ssd"
    image_type      = "COS_CONTAINERD"

    metadata = {
      disable-legacy-endpoints = "true"
    }

    oauth_scopes = [
      "https://www.googleapis.com/auth/devstorage.read_only",
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
      "https://www.googleapis.com/auth/service.management.readonly",
      "https://www.googleapis.com/auth/servicecontrol",
      "https://www.googleapis.com/auth/trace.append",
    ]

    shielded_instance_config {
      enable_secure_boot          = true
      enable_integrity_monitoring = true
    }
  }
}

resource "google_container_node_pool" "high_mem_pool" {
  name     = "high-mem-pool"
  cluster  = google_container_cluster.multizone_cluster.name
  location = google_container_cluster.multizone_cluster.location

  node_locations = ["us-west1-a", "us-west1-b"]

  # This pool has a fixed size.
  node_count = 1

  management {
    auto_repair  = true
    auto_upgrade = false # Note: Auto-upgrade is disabled for this pool.
  }

  max_pods_per_node = 110

  node_config {
    machine_type    = "e2-highmem-4"
    disk_size_gb    = 200
    disk_type       = "pd-ssd"
    image_type      = "COS_CONTAINERD"

    # This taint prevents general workloads from being scheduled on this pool.
    taint {
      key    = "workload"
      value  = "memory-intensive"
      effect = "NO_SCHEDULE"
    }

    metadata = {
      disable-legacy-endpoints = "true"
    }

    oauth_scopes = [
      "https://www.googleapis.com/auth/devstorage.read_only",
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
      "https://www.googleapis.com/auth/service.management.readonly",
      "https://www.googleapis.com/auth/servicecontrol",
      "https://www.googleapis.com/auth/trace.append",
    ]
  }
}
