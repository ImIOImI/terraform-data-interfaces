package main

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

func TestReadConfig(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Valid config file
	validConfig := `
shell: bash
projects:
  - path: /example/path
command: terraform
`
	afero.WriteFile(fs, "valid_config.yaml", []byte(validConfig), 0644)
	config, err := readConfig(fs, "valid_config.yaml")
	assert.Nil(t, err)
	assert.Equal(t, "bash", config.Shell)
	assert.Equal(t, "/example/path", config.Projects[0].Path)
	assert.Equal(t, "terraform", config.Command)

	// Config file with missing fields
	missingFieldsConfig := `
projects:
  - path: /example/path
`
	afero.WriteFile(fs, "missing_fields_config.yaml", []byte(missingFieldsConfig), 0644)
	config, err = readConfig(fs, "missing_fields_config.yaml")
	assert.Nil(t, err)
	assert.Equal(t, "", config.Shell)
	assert.Equal(t, "/example/path", config.Projects[0].Path)

	// Invalid YAML format
	invalidYAML := `
shell: bash
projects
  - path: /example/path
`
	afero.WriteFile(fs, "invalid_yaml_config.yaml", []byte(invalidYAML), 0644)
	_, err = readConfig(fs, "invalid_yaml_config.yaml")
	assert.NotNil(t, err)

	// Empty config file
	afero.WriteFile(fs, "empty_config.yaml", []byte(""), 0644)
	_, err = readConfig(fs, "empty_config.yaml")
	assert.NotNil(t, err)
}

func TestFindAnnotatedOutputs(t *testing.T) {
	fs := afero.NewMemMapFs()

	// File with four outputs, none annotated
	content1 := `
output "output1" {
  value = "value1"
}
output "output2" {
  value = "value2"
}
output "output3" {
  value = "value3"
}
output "output4" {
  value = "value4"
}
`
	afero.WriteFile(fs, "/test1.tf", []byte(content1), 0644)
	outputs := findAnnotatedOutputs(fs, "/", false)
	assert.Equal(t, 0, len(outputs))

	// File with four outputs, one annotated
	content2 := `
# @public
output "output1" {
  value = "value1"
}
output "output2" {
  value = "value2"
}
output "output3" {
  value = "value3"
}
output "output4" {
  value = "value4"
}
`
	afero.WriteFile(fs, "/test2.tf", []byte(content2), 0644)
	outputs = findAnnotatedOutputs(fs, "/", false)
	assert.Equal(t, 1, len(outputs))
	assert.Equal(t, "output1", outputs[0].Output)
	assert.Equal(t, "value1", outputs[0].Reference)

	// File with four outputs, three annotated
	content3 := `
# @public
output "output1" {
  value = "value1"
}
# @public
output "output2" {
  value = "value2"
}
# @public
output "output3" {
  value = "value3"
}
output "output4" {
  value = "value4"
}
`
	afero.WriteFile(fs, "/test3.tf", []byte(content3), 0644)
	outputs = findAnnotatedOutputs(fs, "/", false)
	assert.Equal(t, 3, len(outputs))
	assert.Equal(t, "output1", outputs[0].Output)
	assert.Equal(t, "value1", outputs[0].Reference)
	assert.Equal(t, "output2", outputs[1].Output)
	assert.Equal(t, "value2", outputs[1].Reference)
	assert.Equal(t, "output3", outputs[2].Output)
	assert.Equal(t, "value3", outputs[2].Reference)

	// File with one output, one annotated
	content4 := `
# @public
output "output1" {
  value = "value1"
}
`
	afero.WriteFile(fs, "/test4.tf", []byte(content4), 0644)
	outputs = findAnnotatedOutputs(fs, "/", false)
	assert.Equal(t, 1, len(outputs))
	assert.Equal(t, "output1", outputs[0].Output)
	assert.Equal(t, "value1", outputs[0].Reference)

	// File with one output, none annotated
	content5 := `
output "output1" {
  value = "value1"
}
`
	afero.WriteFile(fs, "/test5.tf", []byte(content5), 0644)
	outputs = findAnnotatedOutputs(fs, "/", false)
	assert.Equal(t, 0, len(outputs))

	// File with mixed content
	content6 := `
resource "random_pet" "my_random_pet" {
  length = 2
  separator = "-"
}
# @public
output "output1" {
  value = random_pet.my_random_pet.id
}
resource "random_string" "my_random_string" {
  length  = 16
  special = false
}
# @public
output "output3" {
  value = random_string.my_random_string.result
}
`
	afero.WriteFile(fs, "/test6.tf", []byte(content6), 0644)
	outputs = findAnnotatedOutputs(fs, "/", false)
	assert.Equal(t, 2, len(outputs))
	assert.Equal(t, "output1", outputs[0].Output)
	assert.Equal(t, "random_pet.my_random_pet.id", outputs[0].Reference)
	assert.Equal(t, "output3", outputs[1].Output)
	assert.Equal(t, "random_string.my_random_string.result", outputs[1].Reference)

	// Empty file
	content7 := ``
	afero.WriteFile(fs, "/test7.tf", []byte(content7), 0644)
	outputs = findAnnotatedOutputs(fs, "/", false)
	assert.Equal(t, 0, len(outputs))

	// File with multiple @public annotations but incomplete output blocks
	content8 := `
# @public
output "output1" {
  value = "value1"
}
# @public
output "output2" {
  value = "value2"
}
`
	afero.WriteFile(fs, "/test8.tf", []byte(content8), 0644)
	outputs = findAnnotatedOutputs(fs, "/", false)
	assert.Equal(t, 2, len(outputs))
	assert.Equal(t, "output1", outputs[0].Output)
	assert.Equal(t, "value1", outputs[0].Reference)
	assert.Equal(t, "output2", outputs[1].Output)
	assert.Equal(t, "value2", outputs[1].Reference)
}

func TestFilterValidOutputs(t *testing.T) {
	schema := ProviderSchema{
		ProviderSchemas: map[string]ProviderSchemaDetails{
			"provider1": {
				ResourceSchemas: map[string]ResourceSchema{
					"resource1": {},
				},
				DataSourceSchemas: map[string]ResourceSchema{
					"data1": {},
				},
			},
			"provider2": {
				ResourceSchemas: map[string]ResourceSchema{
					"resource2": {},
				},
				DataSourceSchemas: map[string]ResourceSchema{
					"data2": {},
				},
			},
		},
	}
	state := TerraformState{}

	t.Run("No valid outputs", func(t *testing.T) {
		outputs := []AnnotatedOutput{
			{Output: "output1", Reference: "invalid_resource1.instance1"},
		}
		validOutputs := filterValidOutputs(outputs, schema, state, false)
		assert.Equal(t, 0, len(validOutputs))
	})

	t.Run("Some valid outputs", func(t *testing.T) {
		outputs := []AnnotatedOutput{
			{Output: "output1", Reference: "resource1.instance1"},
			{Output: "output2", Reference: "invalid_resource2.instance2"},
		}
		validOutputs := filterValidOutputs(outputs, schema, state, false)
		assert.Equal(t, 1, len(validOutputs))
		assert.Equal(t, "output1", validOutputs[0].Output)
	})

	t.Run("All valid outputs", func(t *testing.T) {
		outputs := []AnnotatedOutput{
			{Output: "output1", Reference: "resource1.instance1"},
			{Output: "output2", Reference: "resource2.instance2"},
		}
		validOutputs := filterValidOutputs(outputs, schema, state, false)
		assert.Equal(t, 2, len(validOutputs))
		assert.Equal(t, "output1", validOutputs[0].Output)
		assert.Equal(t, "output2", validOutputs[1].Output)
	})
}

func TestIntegration(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Setup a mock Terraform project
	projectPath := "/project"
	afero.WriteFile(fs, projectPath+"/main.tf", []byte(`
resource "resource1" "instance1" {
  attribute1 = "value1"
}
resource "resource2" "instance2" {
  attribute1 = "value2"
}
# @public
output "output1" {
  value = resource1.instance1.attribute1
}
# @public
output "output2" {
  value = resource2.instance2.attribute1
}
# Not annotated output
output "output3" {
  value = "value3"
}
`), 0644)

	// Mock Terraform state
	state := TerraformState{
		Values: struct {
			RootModule struct {
				Resources []struct {
					Address string                 `json:"address"`
					Values  map[string]interface{} `json:"values"`
				} `json:"resources"`
				Outputs map[string]struct {
					Value interface{} `json:"value"`
				} `json:"outputs"`
			} `json:"root_module"`
		}{
			RootModule: struct {
				Resources []struct {
					Address string                 `json:"address"`
					Values  map[string]interface{} `json:"values"`
				} `json:"resources"`
				Outputs map[string]struct {
					Value interface{} `json:"value"`
				} `json:"outputs"`
			}{
				Resources: []struct {
					Address string                 `json:"address"`
					Values  map[string]interface{} `json:"values"`
				}{
					{
						Address: "resource1.instance1",
						Values: map[string]interface{}{
							"attribute1": "value1",
						},
					},
					{
						Address: "resource2.instance2",
						Values: map[string]interface{}{
							"attribute1": "value2",
						},
					},
				},
			},
		},
	}

	// Mock provider schema
	schema := ProviderSchema{
		ProviderSchemas: map[string]ProviderSchemaDetails{
			"provider1": {
				ResourceSchemas: map[string]ResourceSchema{
					"resource1": {},
				},
				DataSourceSchemas: map[string]ResourceSchema{
					"data1": {},
				},
			},
			"provider2": {
				ResourceSchemas: map[string]ResourceSchema{
					"resource2": {},
				},
				DataSourceSchemas: map[string]ResourceSchema{
					"data2": {},
				},
			},
		},
	}

	// Run the integration test
	outputs := findAnnotatedOutputs(fs, projectPath, false)
	validOutputs := filterValidOutputs(outputs, schema, state, false)
	assert.Equal(t, 2, len(validOutputs))
	assert.Equal(t, "output1", validOutputs[0].Output)
	assert.Equal(t, "output2", validOutputs[1].Output)
}

func TestProcessProject(t *testing.T) {
	fs := afero.NewMemMapFs()
	currentDir := "/"
	shell := "bash"
	command := "terraform"
	verbose := false

	// Valid project
	project := ProjectConfig{Path: "valid_project"}
	fs.MkdirAll("/valid_project", 0755)
	afero.WriteFile(fs, "/valid_project/main.tf", []byte(`
resource "resource1" "instance1" {
  attribute1 = "value1"
}
resource "resource2" "instance2" {
  attribute1 = "value2"
}
# @public
output "output1" {
  value = resource1.instance1.attribute1
}
# @public
output "output2" {
  value = resource2.instance2.attribute1
}
`), 0644)

	err := processProject(fs, project, currentDir, shell, command, verbose)
	assert.Nil(t, err)
	assert.FileExists(t, "/valid_project/interface/generated_data.tf")
	assert.FileExists(t, "/valid_project/interface/generated_providers.tf")
	assert.FileExists(t, "/valid_project/interface/generated_outputs.tf")

	// Non-existent project path
	project = ProjectConfig{Path: "non_existent_project"}
	err = processProject(fs, project, currentDir, shell, command, verbose)
	assert.NotNil(t, err)

	// No annotated outputs
	project = ProjectConfig{Path: "no_annotated_outputs_project"}
	fs.MkdirAll("/no_annotated_outputs_project", 0755)
	afero.WriteFile(fs, "/no_annotated_outputs_project/main.tf", []byte(`
output "output1" {
  value = "value1"
}
`), 0644)
	err = processProject(fs, project, currentDir, shell, command, verbose)
	assert.Nil(t, err)
}
