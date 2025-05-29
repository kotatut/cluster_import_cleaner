resource "null_resource" "example" {
  name = "my-resource"
}

resource "random_pet" "server" {
  prefix = "test-"
  length = 2
  name   = "another-resource" # This should also be modified
}

module "my_module" {
  source = "./my-module"
  name   = "module-level-name" // This should also be modified
}

# This name should not be modified as it's not an attribute of a resource
variable "name" {
  type    = string
  default = "variable-name"
}
