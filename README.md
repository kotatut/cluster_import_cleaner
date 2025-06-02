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

## Troubleshooting

### Build Error: `cty.Type does not implement fmt.Stringer`

If you encounter a build error similar to this:
```
hclmodifier/modifier.go:687:142: cannot use autopilotVal.Type() (value of struct type cty.Type) as fmt.Stringer value in argument to zap.Stringer: cty.Type does not implement fmt.Stringer (missing method String)
```
This error indicates that a `cty.Type` object (from HashiCorp's HCL library) was likely passed to a logging function (like `zap.Stringer` or `zap.Any`) without being properly converted to a string. `cty.Type` itself does not have a `String()` method that `zap` can use directly for logging.

**Potential Causes and Solutions:**

1.  **Outdated Dependencies:** Your local Go module dependencies might be outdated or incompatible.
    *   **Solution:** Try updating your dependencies:
        ```bash
        go get -u all
        ```

2.  **Build Cache Issues:** The Go build cache might contain stale information.
    *   **Solution:** Clean the build cache:
        ```bash
        go clean -cache
        ```

3.  **Go Version or Tooling:** There might be an incompatibility with your Go version or other build tools.
    *   **Solution:** Ensure you are using a compatible Go version (as specified in prerequisites or try a recent stable version).

4.  **Code Issues (If Developing):** If you are modifying the code, ensure that any `cty.Type` values being logged are first converted to a string representation. The `cty.Type` provides a `FriendlyName()` method for this purpose.
    *   **Example:**
        ```go
        // Assuming 'autopilotVal' is a cty.Value and 'logger' is a zap.Logger
        // Incorrect:
        // logger.Info("Autopilot type", zap.Stringer("type", autopilotVal.Type())) // This would cause the error

        // Correct:
        logger.Info("Autopilot type", zap.String("type", autopilotVal.Type().FriendlyName()))
        ```

If the error persists after trying these steps, consider reviewing recent code changes or the specific context where `cty.Type` values are handled or logged.
