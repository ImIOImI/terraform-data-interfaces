# Configure the random provider
provider "random" {}

# Configure the local provider
provider "local" {}

# Create a random pet name
resource "random_pet" "my_random_pet" {
  length    = 2
  separator = "-"
}

resource "random_string" "my_random_string" {
  length  = 16
  special = false
}

# Create a local file with the random string
resource "local_file" "my_local_file" {
  content  = random_string.my_random_string.result
  filename = "${path.module}/random_string.txt"
}

# Output the random pet name
output "random_pet_name" {
  value = random_pet.my_random_pet.id
}

# Output the random string
output "random_string_value" {
  value = random_string.my_random_string.result
}

# Output the path to the local file
output "local_file_path" {
  value = local_file.my_local_file.filename
}