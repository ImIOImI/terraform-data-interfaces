Certainly! Here is the list of requirements written in Markdown:

## Requirements for Terraform Project Automation Script

1. **Configuration:**
    - The script should read a configuration file (`config.yaml`) that specifies:
        - The shell to use for executing commands (`shell`).
        - The paths to Terraform projects (`terraform_project_paths`).
        - Whether to use `tofu` instead of `terraform` (`use_tofu`).
        - Whether to enable verbose output (`verbose`).

2. **Annotated Outputs:**
    - The script should search for annotated outputs in the Terraform files. Annotations are marked with `@public`.
    - Example annotation:
        ```hcl
        # Output the path to the local file
        # @public
        output "local_file_path" {
          value = local_file.my_local_file.filename
        }
        ```

3. **Resource State:**
    - For each annotated output, the script should:
        - Find the resource the output refers to.
        - Fetch the state of the resource.
        - Extract the required attributes from the resource state.

4. **Data Sources:**
    - The script should check if there is a matching data resource for each annotated resource.
    - If a matching data resource exists, the script should:
        - Create a folder named `interface` in the Terraform project directory.
        - Generate a `generated_data.tf` file with data source blocks for each unique annotated resource.
        - Ensure the data source blocks include the required attributes from the resource state.

5. **Providers:**
    - The script should generate a `generated_providers.tf` file that specifies the required providers for the new Terraform module in the `interface` folder.

6. **Logging:**
    - The script should log the following items in green color:
        - The unique list of data sources found in a Terraform project.
        - Each data source's required attributes.
        - The matching resource.
        - The resource's state.
    - If an annotated resource does not have a matching data resource, the script should log a red warning:
        ```
        Annotated resource <resource name> at line <line number> in file <filename> does not have a matching data resource!
        ```

7. **Error Handling:**
    - If the Terraform project is not applied, the script should log an error and skip the project.

8. **Commands:**
    - The script should use the correct command (`terraform` or `tofu`) based on the configuration.
    - Example commands:
        ```bash
        terraform show -json
        tofu show -json
        terraform providers schema -json
        tofu providers schema -json
        ```

These requirements ensure the script performs the necessary operations to automate the process of generating data sources and provider configurations for annotated outputs in Terraform projects.