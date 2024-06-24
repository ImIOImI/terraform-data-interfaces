## Requirements for Terraform Project Automation Script

1. **Configuration:**
   The script should read a configuration file (`config.yaml`)
   
   ***Schema***
   ```
   shell: <string>  # Optional
   projects:        # Required
   - path: <string>  # Required
     generated_folder_name: <string>  # Optional
     generated_folder_path: <string>  # Optional
    default_command: <string>  # Required
    verbose: <bool>  # Optional
    ```
    ***Descriptions***
    - shell: 
      - Description: The shell to use for executing commands (e.g., bash or zsh). 
      - Required: `false`
      - Type: string
      - Default: "bash"
    - projects: 
      - Description: List of projects with specific configurations.
      - Required: true
      - Type: list(object)
          - path: 
            - Description: path to terraform/tofu project. Relative to where the go executable was ran
            - Type: string
            - Required: true
          - generatedFolderName: 
            - Description: Name of the folder to generate 
            - Required: `false` 
            - Type: string
            - Default: "interface" 
          - generatedFolderPath:
             - Description: Path where the generated folder will be located. Relative to where the go executable was ran
             - Required: `false`
             - Type: string
             - Default: defaults to the terraform/tofu project path 
    - command:
      - Description: command to use to call terraform/tofu
      - Required: `false`
      - Type: string (tofu, terraform, terramate... etc)
      - Default: "terraform" 
    - verbose: 
      - Description: Enable verbose output (default is false).
      - Required: `false`
      - Type: `bool`
      - Default: `false`

2. **Annotated Outputs:**
    - The script should search for annotated outputs in the Terraform files. Annotations are marked with `@public` in 
      the comments.
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
        - Create outputs with the same names as those found in requirement 3, 
          - each output should refer to the relevant data source. 
          - the outputs should be generated in the file `generated_outputs.tf`

5. **Providers:**
    - The script should generate a `generated_providers.tf` file that specifies the required providers for the new 
      Terraform module in the `interface` folder.
    - Only the necessary providers for the datasources in `generated_data.tf` should be generated

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
9. **Testable**
    - The script must be able to be unit tested