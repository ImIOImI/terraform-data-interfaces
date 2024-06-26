# Terraform Data Interfaces
This is currently a WIP project. The goal is to be able to annotate outputs with comments, and build an interface 
module using autogenerated code that creates data sources with the same outputs. This module can be shared freely 
amongst other teams and projects to share public configuration data. 

## How it works
The provider schema and state is downloaded for a terraform/tofu project. The code is scanned for annotations on 
outputs. If one is found, we'll check to see if there is an equivalent data source available. If there isn't one, it'll   
throw a warning and move on. If there is, it'll find the required attributes for the data source and check against the
resource's state for equivalent inputs. From there, the script autogenerates a module in a folder called "interface" 
with the data sources, outputs and provider details.  

## Quickstart

### Annotate Your Outputs
Anything that you want to be "public" you should annotate with the `@public` tag in the comments

```terraform
# Output the path to the local file
# @public
output "local_file_path" {
  description = "Filename of the local file"
  value       = local_file.my_local_file.filename
}
```
**_NOTE:_**  You **MUST** only use comments with the `#` symbol, currently. Sorry. I expect to solve this in a future 
release

### Create a config.yaml file 
You can pass flags to the script, but setting up a config.yaml file is the easiest way to repeatedly scan a 
terraform/tofu project

```terraform
shell: zsh
terraform_project_paths:
  - examples/multiple-resources-no-annotations
  - examples/multiple-resources-one-annotation
  - examples/multiple-resources-multiple-annotations
use_tofu: true
#verbose: true
```

### Run the script
After the terraform/tofu project has been applied, run the script. A new folder called `interface` will be created in 
the terraform/tofu project with the files:
```shell
generated_data.tf
generated_outputs.tf
generated_providers.tf
```
If files by those names already exist, they will be replaced and the new content will be added.

### Call the Interface Module

From there, all you need to do is call the interface module and use the outputs it generates


## Dependencies
You must have either OpenTofu or Terraform installed. I highly recommend that you install [tenv](https://github.com/tofuutils/tenv) (a version manager written 
to manage both OpenTofu and Terraform)

