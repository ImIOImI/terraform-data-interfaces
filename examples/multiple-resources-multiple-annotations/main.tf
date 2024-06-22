# Configure the random provider
provider "random" {}

# Configure the local provider
provider "local" {}

# Create a random pet name
#
#  even more words
resource "random_pet" "my_random_pet" {
  length    = 2
  separator = "-"
}

#  Create a random string
resource "random_string" "my_random_string" {
  length  = 16
  special = false
}


#  Create a local file with the random string
#
#  more words
resource "local_file" "my_local_file" {
  content  = random_string.my_random_string.result
  filename = "${path.module}/random_string.txt"
}

# Output the random pet name
output "random_pet_name" {
  value = random_pet.my_random_pet.id
}

# Output the random string
# @public
output "random_string_value" {
  value = random_string.my_random_string.result
}

# Output the random string
# @public
# words
output "random_string_value1" {
  value = random_string.my_random_string.result
}

# Output the random string
# @public
# more words
output "random_string_value2" {
  value = random_string.my_random_string.result
}

# Output the path to the local file
# @public
output "local_file_path" {
  description = "test output with description"
  value       = local_file.my_local_file.filename
}

# @public
output "local_file_contents" {
  description = ""
  value = local_file.my_local_file.content
}

# words
# @public
# More words
output "local_file_contents_base64" {
  value = local_file.my_local_file.content_base64
}
