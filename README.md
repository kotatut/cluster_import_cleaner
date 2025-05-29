# Terraform Modifier (tf-modifier)

`tf-modifier` is a command-line tool written in Go that parses a Terraform (.tf) file, modifies it, and saves the changes. Specifically, it finds all resource attributes named "name" and appends the suffix "-clone" to their string values.

## Prerequisites

- Go (version 1.18 or later recommended)

## Building the Tool

To build the `tf-modifier` tool, navigate to the root directory of the project and run:

```bash
go build -o tf-modifier .
```
This will create an executable file named `tf-modifier` in the current directory.

## Usage

To use the `tf-modifier` tool, run the compiled executable with the path to the Terraform file you want to modify as an argument:

```bash
./tf-modifier path/to/your/terraform_file.tf
```

For example, you can use the provided `test.tf` file to see how it works:

```bash
./tf-modifier test.tf
```
After execution, the `test.tf` file will be modified in place. The original values of the "name" attributes will have "-clone" appended to them.

## Example

The `test.tf` file is included in this repository as an example:

```terraform
resource "null_resource" "example" {
  name = "my-resource"
}

resource "random_pet" "server" {
  prefix = "test-"
  length = 2
  name   = "another-resource"
}

module "my_module" {
  source = "./my-module" # Assuming a local module for example purposes
  name   = "module-level-name"
}

variable "name" {
  type    = string
  default = "variable-name"
}
```

After running `./tf-modifier test.tf`, the `test.tf` file will be updated to:

```terraform
resource "null_resource" "example" {
  name = "my-resource-clone"
}

resource "random_pet" "server" {
  prefix = "test-"
  length = 2
  name   = "another-resource-clone"
}

module "my_module" {
  source = "./my-module"
  name   = "module-level-name-clone"
}

variable "name" {
  type    = string
  default = "variable-name"
}
```

## Logging

The tool uses `go.uber.org/zap` for structured logging. Logs are output to standard error.
```
