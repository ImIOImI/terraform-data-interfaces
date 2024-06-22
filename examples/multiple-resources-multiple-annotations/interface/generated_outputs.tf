output "local_file_path" {
  value = data.local_file.my_local_file.filename
}

output "local_file_contents" {
  value = data.local_file.my_local_file.content
}

output "local_file_contents_base64" {
  value = data.local_file.my_local_file.content_base64
}

