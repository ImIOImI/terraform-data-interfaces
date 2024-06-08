package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"
)

// Config represents the structure of the config file.
type Config struct {
	Shell                string `yaml:"shell"`
	TerraformProjectPath string `yaml:"terraform_project_path"`
	UseTofu              bool   `yaml:"use_tofu"`
	Verbose              bool   `yaml:"verbose"`
}

// ProviderSchema represents the structure of the schema JSON output.
type ProviderSchema struct {
	FormatVersion   string                         `json:"format_version"`
	ProviderSchemas map[string]ProviderSchemaEntry `json:"provider_schemas"`
}

type ProviderSchemaEntry struct {
	Provider          ProviderDetails             `json:"provider"`
	ResourceSchemas   map[string]ResourceSchema   `json:"resource_schemas"`
	DataSourceSchemas map[string]DataSourceSchema `json:"data_source_schemas"`
}

type ProviderDetails struct {
	Version int   `json:"version"`
	Block   Block `json:"block"`
}

type Block struct {
	DescriptionKind string `json:"description_kind"`
}

type ResourceSchema struct {
	Version int           `json:"version"`
	Block   ResourceBlock `json:"block"`
}

type ResourceBlock struct {
	Attributes      map[string]Attribute `json:"attributes"`
	Description     string               `json:"description"`
	DescriptionKind string               `json:"description_kind"`
}

type DataSourceSchema struct {
	Version int             `json:"version"`
	Block   DataSourceBlock `json:"block"`
}

type DataSourceBlock struct {
	Attributes      map[string]Attribute `json:"attributes"`
	Description     string               `json:"description"`
	DescriptionKind string               `json:"description_kind"`
}

// Custom Type to handle both string and array for the type field
type Type struct {
	Value interface{}
}

func (t *Type) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		t.Value = single
		return nil
	}
	var array []string
	if err := json.Unmarshal(data, &array); err == nil {
		t.Value = array
		return nil
	}
	return fmt.Errorf("type should be a string or an array of strings")
}

type Attribute struct {
	Type            Type   `json:"type"`
	Description     string `json:"description"`
	DescriptionKind string `json:"description_kind"`
	Optional        bool   `json:"optional,omitempty"`
	Computed        bool   `json:"computed,omitempty"`
	Required        bool   `json:"required,omitempty"`
	Sensitive       bool   `json:"sensitive,omitempty"`
	Deprecated      bool   `json:"deprecated,omitempty"`
}

// Function to check for @interface annotations and extract resource type and name
func findAnnotatedResources(path string) {
	var annotatedResources []string

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(info.Name(), ".tf") || strings.HasSuffix(info.Name(), ".tf.json")) {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			inAnnotation := false
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
					if strings.Contains(line, "@interface") {
						inAnnotation = true
					}
				} else if inAnnotation && (strings.HasPrefix(line, "resource")) {
					resourceInfo := extractResourceInfo(line)
					if resourceInfo != "" {
						annotatedResources = append(annotatedResources, resourceInfo)
						inAnnotation = false
					}
				} else {
					inAnnotation = false
				}
			}
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Failed to walk through the Terraform project path: %v", err)
	}

	if len(annotatedResources) == 0 {
		log.Println("No resources annotated with @interface found.")
	} else {
		log.Println("Annotated Resources:")
		for _, resource := range annotatedResources {
			fmt.Println(resource)
		}
	}
}

// Function to extract resource type and name
func extractResourceInfo(line string) string {
	re := regexp.MustCompile(`resource\s+"(\w+)"\s+"(\w+)"`)
	matches := re.FindStringSubmatch(line)
	if len(matches) == 3 {
		return fmt.Sprintf("Resource Type: %s, Name: %s", matches[1], matches[2])
	}
	return ""
}

func main() {
	// Define the flags for shell choice, terraform project path, using tofu, and verbose
	shellFlag := flag.String("shell", "", "Shell to use for executing commands")
	projectPathFlag := flag.String("project-path", "", "Path to the Terraform project")
	useTofuFlag := flag.Bool("use-tofu", false, "Use tofu instead of terraform for commands")
	verboseFlag := flag.Bool("verbose", false, "Enable verbose output")
	flag.Parse()

	// Read the config file
	config := Config{}
	configFile, err := ioutil.ReadFile("config.yaml")
	if err == nil {
		if err := yaml.Unmarshal(configFile, &config); err != nil {
			log.Fatalf("Failed to parse config file: %v", err)
		}
	}

	// Determine which shell to use
	shell := "bash"
	if *shellFlag != "" {
		shell = *shellFlag
	} else if config.Shell != "" {
		shell = config.Shell
	}

	// Determine the path to the Terraform project
	terraformProjectPath := "."
	if *projectPathFlag != "" {
		terraformProjectPath = *projectPathFlag
	} else if config.TerraformProjectPath != "" {
		terraformProjectPath = config.TerraformProjectPath
	}

	// Determine whether to use tofu or terraform
	useTofu := false
	if *useTofuFlag {
		useTofu = *useTofuFlag
	} else if config.UseTofu {
		useTofu = config.UseTofu
	}

	// Determine verbose output
	verbose := false
	if *verboseFlag {
		verbose = *verboseFlag
	} else if config.Verbose {
		verbose = config.Verbose
	}

	// Output selected options
	log.Printf("Using shell: %s", shell)
	log.Printf("Terraform project path: %s", terraformProjectPath)
	log.Printf("Using tofu: %v", useTofu)
	log.Printf("Verbose output: %v", verbose)

	// Change directory to the Terraform project path
	if err := os.Chdir(terraformProjectPath); err != nil {
		log.Fatalf("Failed to change directory to Terraform project: %v", err)
	}
	log.Printf("Changed directory to Terraform project: %s", terraformProjectPath)

	// Find and print annotated resources
	findAnnotatedResources(".")

	// Determine the command to execute
	var cmd *exec.Cmd
	if useTofu {
		cmd = exec.Command(shell, "-c", "tofu providers schema -json")
	} else {
		cmd = exec.Command(shell, "-c", "terraform providers schema -json")
	}
	cmd.Env = append(os.Environ(), fmt.Sprintf("PATH=%s", os.Getenv("PATH"))) // Ensure PATH is inherited

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Failed to run command: %v", err)
		log.Printf("Command output: %s", output)
		log.Fatalf("Exiting due to command failure")
	}

	// Attempt to parse the JSON output
	var schema ProviderSchema
	err = json.Unmarshal(output, &schema)
	if err != nil {
		log.Printf("Failed to parse JSON output: %v", err)
		log.Printf("Command output: %s", output)
		log.Fatalf("Exiting due to unmarshalling error")
	}

	// Print the parsed schema if verbose is enabled
	if verbose {
		fmt.Printf("Provider Schema: %+v\n", schema)
	}
}
