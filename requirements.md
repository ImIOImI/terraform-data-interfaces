## Requirements for Terraform Project Automation Script

1. **Configuration:**
   The script should read a configuration file (`config.yaml`)
   
   ***Schema***
   ```
   shell: <string>  # Required
   projects:        # Required
   - path: <string>  # Required
     generated_folder_name: <string>  # Optional
     generated_folder_path: <string>  # Optional
     command: <string>  # Optional
    default_command: <string>  # Required
    verbose: <bool>  # Optional
    ```
    ***Descriptions***
    - shell: The shell to use for executing commands (e.g., bash or zsh). Defaults to bash
    - projects: List of projects with specific configurations.
        - path: 
        - generated_folder_name: Name of the folder to generate (default is interface).
        - generated_folder_path: Path where the generated folder will be located (default is the project path).
        - command: project-specific command to use if a project-specific command is not provided (default is terraform)
    - default_command: Description: Default command to use if a project-specific command is not provided (default is terraform)
    - verbose: Enable verbose output (default is false).

3. **Annotated Outputs:**
    - The script should search for annotated outputs in the Terraform files. Annotations are marked with `@public`.
    - Example annotation:
        ```hcl
        # Output the path to the local file
        # @public
        output "local_file_path" {
          value = local_file.my_local_file.filename
        }
        ```

4. **Resource State:**
    - For each annotated output, the script should:
        - Find the resource the output refers to.
        - Fetch the state of the resource.
        - Extract the required attributes from the resource state.

5. **Data Sources:**
    - The script should check if there is a matching data resource for each annotated resource.
    - If a matching data resource exists, the script should:
        - Create a folder named `interface` in the Terraform project directory.
        - Generate a `generated_data.tf` file with data source blocks for each unique annotated resource.
        - Ensure the data source blocks include the required attributes from the resource state.

6. **Providers:**
    - The script should generate a `generated_providers.tf` file that specifies the required providers for the new Terraform module in the `interface` folder.

7. **Logging:**
    - The script should log the following items in green color:
        - The unique list of data sources found in a Terraform project.
        - Each data source's required attributes.
        - The matching resource.
        - The resource's state.
    - If an annotated resource does not have a matching data resource, the script should log a red warning:
        ```
        Annotated resource <resource name> at line <line number> in file <filename> does not have a matching data resource!
        ```

8. **Error Handling:**
    - If the Terraform project is not applied, the script should log an error and skip the project.

9. **Commands:**
    - The script should use the correct command (`terraform` or `tofu`) based on the configuration.
    - Example commands:
        ```bash
        terraform show -json
        tofu show -json
        terraform providers schema -json
        tofu providers schema -json
        ```
