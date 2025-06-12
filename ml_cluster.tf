# __generated__ by Terraform
# Please review these resources and move them into your main configuration files.

# __generated__ by Terraform
resource "google_container_cluster" "rag-cluster" {
  allow_net_admin                          = null
  cluster_ipv4_cidr                        = "10.52.0.0/14"
  datapath_provider                        = "ADVANCED_DATAPATH"
  default_max_pods_per_node                = 110
  deletion_protection                      = true
  description                              = null
  disable_l4_lb_firewall_reconciliation    = false
  enable_autopilot                         = false
  enable_cilium_clusterwide_network_policy = false
  enable_fqdn_network_policy               = false
  enable_intranode_visibility              = false
  enable_kubernetes_alpha                  = false
  enable_l4_ilb_subsetting                 = false
  enable_legacy_abac                       = false
  enable_multi_networking                  = false
  enable_shielded_nodes                    = true
  enable_tpu                               = false
  in_transit_encryption_config             = null
  initial_node_count                       = 0
  location                                 = "us-central1"
  logging_service                          = "logging.googleapis.com/kubernetes"
  min_master_version                       = null
  monitoring_service                       = "monitoring.googleapis.com/kubernetes"
  name                                     = "rag-cluster"
  network                                  = "projects/steel-thread-clusters/global/networks/ml-network"
  networking_mode                          = "VPC_NATIVE"
  node_locations                           = ["us-central1-a", "us-central1-b", "us-central1-c"]
  node_version                             = "1.30.12-gke.1151000"
  private_ipv6_google_access               = null
  project                                  = "steel-thread-clusters"
  remove_default_node_pool                 = null
  resource_labels                          = {}
  subnetwork                               = "projects/steel-thread-clusters/regions/us-central1/subnetworks/ml-network"
  addons_config {
    config_connector_config {
      enabled = false
    }
    dns_cache_config {
      enabled = false
    }
    gce_persistent_disk_csi_driver_config {
      enabled = true
    }
    gcp_filestore_csi_driver_config {
      enabled = false
    }
    gcs_fuse_csi_driver_config {
      enabled = true
    }
    gke_backup_agent_config {
      enabled = false
    }
    horizontal_pod_autoscaling {
      disabled = false
    }
    http_load_balancing {
      disabled = false
    }
    network_policy_config {
      disabled = true
    }
    ray_operator_config {
      enabled = true
      ray_cluster_logging_config {
        enabled = true
      }
      ray_cluster_monitoring_config {
        enabled = true
      }
    }
  }
  binary_authorization {
    evaluation_mode = null
  }
  cluster_autoscaling {
    auto_provisioning_locations = []
    autoscaling_profile         = "BALANCED"
    enabled                     = false
  }
  control_plane_endpoints_config {
    dns_endpoint_config {
      allow_external_traffic = false
      endpoint               = "gke-d4107b6615084374864b6a228479419cb218-340607650307.us-central1.gke.goog"
    }
    ip_endpoints_config {
      enabled = true
    }
  }
  database_encryption {
    key_name = null
    state    = "DECRYPTED"
  }
  default_snat_status {
    disabled = false
  }
  enterprise_config {
    desired_tier = null
  }
  ip_allocation_policy {
    cluster_ipv4_cidr_block       = "10.52.0.0/14"
    cluster_secondary_range_name  = "gke-rag-cluster-pods-d4107b66"
    services_ipv4_cidr_block      = "34.118.224.0/20"
    services_secondary_range_name = null
    stack_type                    = "IPV4"
    pod_cidr_overprovision_config {
      disabled = false
    }
  }
  logging_config {
    enable_components = ["SYSTEM_COMPONENTS", "WORKLOADS"]
  }
  maintenance_policy {
    daily_maintenance_window {
      start_time = "05:00"
    }
  }
  master_auth {
    client_certificate_config {
      issue_client_certificate = false
    }
  }
  mesh_certificates {
    enable_certificates = false
  }
  monitoring_config {
    enable_components = ["SYSTEM_COMPONENTS"]
    advanced_datapath_observability_config {
      enable_metrics = false
      enable_relay   = false
    }
    managed_prometheus {
      enabled = true
    }
  }
  network_policy {
    enabled  = false
    provider = "PROVIDER_UNSPECIFIED"
  }
  node_config {
    boot_disk_kms_key           = null
    disk_size_gb                = 100
    disk_type                   = "pd-standard"
    enable_confidential_storage = false
    flex_start                  = false
    image_type                  = "COS_CONTAINERD"
    labels = {
      cluster_name = "rag-cluster"
      created-by   = "ai-on-gke"
      node_pool    = "cpu-pool"
    }
    local_ssd_count           = 0
    local_ssd_encryption_mode = null
    logging_variant           = "DEFAULT"
    machine_type              = "n1-standard-16"
    max_run_duration          = null
    metadata = {
      cluster_name             = "rag-cluster"
      disable-legacy-endpoints = "true"
      node_pool                = "cpu-pool"
    }
    min_cpu_platform = null
    node_group       = null
    oauth_scopes     = ["https://www.googleapis.com/auth/devstorage.read_only", "https://www.googleapis.com/auth/logging.write", "https://www.googleapis.com/auth/monitoring", "https://www.googleapis.com/auth/service.management.readonly", "https://www.googleapis.com/auth/servicecontrol", "https://www.googleapis.com/auth/trace.append"]
    preemptible      = false
    resource_labels = {
      goog-gke-node-pool-provisioning-model = "on-demand"
    }
    resource_manager_tags = {}
    service_account       = "tf-gke-rag-cluster-xciy@steel-thread-clusters.iam.gserviceaccount.com"
    spot                  = false
    storage_pools         = []
    tags                  = ["gke-rag-cluster", "gke-rag-cluster-cpu-pool", "gke-node", "ai-on-gke"]
    gcfs_config {
      enabled = true
    }
    kubelet_config {
      allowed_unsafe_sysctls                 = []
      container_log_max_files                = 0
      container_log_max_size                 = null
      cpu_cfs_quota                          = false
      cpu_cfs_quota_period                   = null
      cpu_manager_policy                     = null
      image_gc_high_threshold_percent        = 0
      image_gc_low_threshold_percent         = 0
      image_maximum_gc_age                   = null
      image_minimum_gc_age                   = null
      insecure_kubelet_readonly_port_enabled = "TRUE"
      pod_pids_limit                         = 0
    }
    shielded_instance_config {
      enable_integrity_monitoring = true
      enable_secure_boot          = false
    }
    windows_node_config {
      osversion = null
    }
    workload_metadata_config {
      mode = "GKE_METADATA"
    }
  }
  node_pool {
    initial_node_count = 1
    max_pods_per_node  = 110
    name               = "cpu-pool"
    name_prefix        = null
    node_count         = 1
    node_locations     = ["us-central1-a", "us-central1-b", "us-central1-c"]
    version            = "1.30.12-gke.1151000"
    autoscaling {
      location_policy      = "BALANCED"
      max_node_count       = 3
      min_node_count       = 1
      total_max_node_count = 0
      total_min_node_count = 0
    }
    management {
      auto_repair  = true
      auto_upgrade = true
    }
    network_config {
      create_pod_range     = false
      enable_private_nodes = false
      pod_ipv4_cidr_block  = "10.52.0.0/14"
      pod_range            = "gke-rag-cluster-pods-d4107b66"
    }
    node_config {
      boot_disk_kms_key           = null
      disk_size_gb                = 100
      disk_type                   = "pd-standard"
      enable_confidential_storage = false
      flex_start                  = false
      image_type                  = "COS_CONTAINERD"
      labels = {
        cluster_name = "rag-cluster"
        created-by   = "ai-on-gke"
        node_pool    = "cpu-pool"
      }
      local_ssd_count           = 0
      local_ssd_encryption_mode = null
      logging_variant           = "DEFAULT"
      machine_type              = "n1-standard-16"
      max_run_duration          = null
      metadata = {
        cluster_name             = "rag-cluster"
        disable-legacy-endpoints = "true"
        node_pool                = "cpu-pool"
      }
      min_cpu_platform = null
      node_group       = null
      oauth_scopes     = ["https://www.googleapis.com/auth/devstorage.read_only", "https://www.googleapis.com/auth/logging.write", "https://www.googleapis.com/auth/monitoring", "https://www.googleapis.com/auth/service.management.readonly", "https://www.googleapis.com/auth/servicecontrol", "https://www.googleapis.com/auth/trace.append"]
      preemptible      = false
      resource_labels = {
        goog-gke-node-pool-provisioning-model = "on-demand"
      }
      resource_manager_tags = {}
      service_account       = "tf-gke-rag-cluster-xciy@steel-thread-clusters.iam.gserviceaccount.com"
      spot                  = false
      storage_pools         = []
      tags                  = ["gke-rag-cluster", "gke-rag-cluster-cpu-pool", "gke-node", "ai-on-gke"]
      gcfs_config {
        enabled = true
      }
      kubelet_config {
        allowed_unsafe_sysctls                 = []
        container_log_max_files                = 0
        container_log_max_size                 = null
        cpu_cfs_quota                          = false
        cpu_cfs_quota_period                   = null
        cpu_manager_policy                     = null
        image_gc_high_threshold_percent        = 0
        image_gc_low_threshold_percent         = 0
        image_maximum_gc_age                   = null
        image_minimum_gc_age                   = null
        insecure_kubelet_readonly_port_enabled = "TRUE"
        pod_pids_limit                         = 0
      }
      shielded_instance_config {
        enable_integrity_monitoring = true
        enable_secure_boot          = false
      }
      windows_node_config {
        osversion = null
      }
      workload_metadata_config {
        mode = "GKE_METADATA"
      }
    }
    upgrade_settings {
      max_surge       = 1
      max_unavailable = 0
      strategy        = "SURGE"
    }
  }
  node_pool {
    initial_node_count = 1
    max_pods_per_node  = 110
    name               = "gpu-pool-l4"
    name_prefix        = null
    node_count         = 1
    node_locations     = ["us-central1-a", "us-central1-b", "us-central1-c"]
    version            = "1.30.12-gke.1151000"
    autoscaling {
      location_policy      = "BALANCED"
      max_node_count       = 3
      min_node_count       = 0
      total_max_node_count = 0
      total_min_node_count = 0
    }
    management {
      auto_repair  = true
      auto_upgrade = true
    }
    network_config {
      create_pod_range     = false
      enable_private_nodes = false
      pod_ipv4_cidr_block  = "10.52.0.0/14"
      pod_range            = "gke-rag-cluster-pods-d4107b66"
    }
    node_config {
      boot_disk_kms_key           = null
      disk_size_gb                = 200
      disk_type                   = "pd-balanced"
      enable_confidential_storage = false
      flex_start                  = false
      image_type                  = "COS_CONTAINERD"
      labels = {
        cluster_name = "rag-cluster"
        created-by   = "ai-on-gke"
        node_pool    = "gpu-pool-l4"
      }
      local_ssd_count           = 0
      local_ssd_encryption_mode = null
      logging_variant           = "DEFAULT"
      machine_type              = "g2-standard-24"
      max_run_duration          = null
      metadata = {
        cluster_name             = "rag-cluster"
        disable-legacy-endpoints = "true"
        node_pool                = "gpu-pool-l4"
      }
      min_cpu_platform = null
      node_group       = null
      oauth_scopes     = ["https://www.googleapis.com/auth/devstorage.read_only", "https://www.googleapis.com/auth/logging.write", "https://www.googleapis.com/auth/monitoring", "https://www.googleapis.com/auth/service.management.readonly", "https://www.googleapis.com/auth/servicecontrol", "https://www.googleapis.com/auth/trace.append"]
      preemptible      = false
      resource_labels = {
        goog-gke-accelerator-type             = "nvidia-l4"
        goog-gke-node-pool-provisioning-model = "on-demand"
      }
      resource_manager_tags = {}
      service_account       = "tf-gke-rag-cluster-xciy@steel-thread-clusters.iam.gserviceaccount.com"
      spot                  = false
      storage_pools         = []
      tags                  = ["gke-rag-cluster", "gke-rag-cluster-gpu-pool-l4", "gke-node", "ai-on-gke"]
      gcfs_config {
        enabled = true
      }
      guest_accelerator {
        count              = 2
        gpu_partition_size = null
        type               = "nvidia-l4"
        gpu_driver_installation_config {
          gpu_driver_version = "DEFAULT"
        }
      }
      kubelet_config {
        allowed_unsafe_sysctls                 = []
        container_log_max_files                = 0
        container_log_max_size                 = null
        cpu_cfs_quota                          = false
        cpu_cfs_quota_period                   = null
        cpu_manager_policy                     = null
        image_gc_high_threshold_percent        = 0
        image_gc_low_threshold_percent         = 0
        image_maximum_gc_age                   = null
        image_minimum_gc_age                   = null
        insecure_kubelet_readonly_port_enabled = "TRUE"
        pod_pids_limit                         = 0
      }
      shielded_instance_config {
        enable_integrity_monitoring = true
        enable_secure_boot          = false
      }
      windows_node_config {
        osversion = null
      }
      workload_metadata_config {
        mode = "GKE_METADATA"
      }
    }
    upgrade_settings {
      max_surge       = 1
      max_unavailable = 0
      strategy        = "SURGE"
    }
  }
  node_pool_defaults {
    node_config_defaults {
      insecure_kubelet_readonly_port_enabled = "FALSE"
      logging_variant                        = "DEFAULT"
    }
  }
  notification_config {
    pubsub {
      enabled = false
      topic   = null
    }
  }
  pod_autoscaling {
    hpa_profile = "HPA_PROFILE_UNSPECIFIED"
  }
  private_cluster_config {
    enable_private_endpoint     = false
    enable_private_nodes        = false
    master_ipv4_cidr_block      = null
    private_endpoint_subnetwork = null
    master_global_access_config {
      enabled = false
    }
  }
  release_channel {
    channel = "REGULAR"
  }
  secret_manager_config {
    enabled = false
  }
  security_posture_config {
    mode               = "DISABLED"
    vulnerability_mode = "VULNERABILITY_DISABLED"
  }
  service_external_ips_config {
    enabled = false
  }
  vertical_pod_autoscaling {
    enabled = false
  }
  workload_identity_config {
    workload_pool = "steel-thread-clusters.svc.id.goog"
  }
}
