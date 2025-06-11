resource "google_container_cluster" "cluster_3_clone_tfer" {
  addons_config {
    dns_cache_config {
      enabled = "false"
    }

    gce_persistent_disk_csi_driver_config {
      enabled = "true"
    }

    gcs_fuse_csi_driver_config {
      enabled = "false"
    }

    horizontal_pod_autoscaling {
      disabled = "false"
    }

    http_load_balancing {
      disabled = "false"
    }

    network_policy_config {
      disabled = "true"
    }
  }

  binary_authorization {
    evaluation_mode = "DISABLED"
  }

  cluster_autoscaling {
    autoscaling_profile = "BALANCED"
    enabled             = "false"
  }


  cluster_telemetry {
    type = "ENABLED"
  }

  control_plane_endpoints_config {
    dns_endpoint_config {
      allow_external_traffic = "false"
      endpoint               = "gke-b2c792b54c20447396db8ff9faf8a470cda9-841846055629.us-central1.gke.goog"
    }
  }

  database_encryption {
    state = "DECRYPTED"
  }

  datapath_provider         = "LEGACY_DATAPATH"
  default_max_pods_per_node = "110"

  default_snat_status {
    disabled = "false"
  }

  enable_cilium_clusterwide_network_policy = "false"
  enable_fqdn_network_policy               = "false"
  enable_intranode_visibility              = "false"
  enable_kubernetes_alpha                  = "false"
  enable_l4_ilb_subsetting                 = "false"
  enable_legacy_abac                       = "false"
  enable_multi_networking                  = "false"
  enable_shielded_nodes                    = "true"
  enable_tpu                               = "false"
  initial_node_count                       = "1"

  ip_allocation_policy {
    cluster_ipv4_cidr_block = "10.114.0.0/14"

    pod_cidr_overprovision_config {
      disabled = "false"
    }

    services_ipv4_cidr_block = "34.118.224.0/20"
    stack_type               = "IPV4"
  }

  location = "us-central1"

  logging_config {
    enable_components = ["SYSTEM_COMPONENTS", "WORKLOADS"]
  }


  master_auth {
    client_certificate_config {
      issue_client_certificate = "false"
    }
  }

  monitoring_config {
    advanced_datapath_observability_config {
      enable_metrics = "false"
      enable_relay   = "false"
    }

    enable_components = ["CADVISOR", "DAEMONSET", "DEPLOYMENT", "HPA", "KUBELET", "POD", "STATEFULSET", "STORAGE", "SYSTEM_COMPONENTS"]

    managed_prometheus {
      enabled = "true"
    }
  }

  name    = "cluster-3-clone-tfer"
  network = "projects/terrakot/global/networks/default"

  network_policy {
    enabled  = "false"
    provider = "PROVIDER_UNSPECIFIED"
  }

  networking_mode = "VPC_NATIVE"
  node_locations  = ["us-central1-a", "us-central1-b", "us-central1-c"]

  node_pool_defaults {
    node_config_defaults {
      insecure_kubelet_readonly_port_enabled = "FALSE"
      logging_variant                        = "DEFAULT"
    }
  }

  node_version = "1.31.4-gke.1372000"

  notification_config {
    pubsub {
      enabled = "false"
    }
  }

  pod_security_policy_config {
    enabled = "false"
  }

  private_cluster_config {
    enable_private_endpoint = "false"
    enable_private_nodes    = "false"

    master_global_access_config {
      enabled = "false"
    }
  }

  project = "terrakot"

  protect_config {
    workload_config {
      audit_mode = "BASIC"
    }

    workload_vulnerability_mode = "DISABLED"
  }

  provider = "google-beta"

  release_channel {
    channel = "REGULAR"
  }

  secret_manager_config {
    enabled = "false"
  }

  security_posture_config {
    mode               = "BASIC"
    vulnerability_mode = "VULNERABILITY_DISABLED"
  }

  service_external_ips_config {
    enabled = "false"
  }

  subnetwork         = "projects/terrakot/regions/us-central1/subnetworks/default"
  min_master_version = "1.31.4-gke.1372000"
}
