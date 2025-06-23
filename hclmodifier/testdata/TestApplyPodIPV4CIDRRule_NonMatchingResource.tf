resource "google_compute_router" "primary" {
  name               = "my-router"
  network            = "default"
  bgp {
    asn = 64514
  }
}
