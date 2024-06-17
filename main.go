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
	Shell                 string   `yaml:"shell"`
	TerraformProjectPaths []string `yaml:"terraform_project_paths"`
	UseTofu               bool     `yaml:"use_tofu"`
	Verbose               bool     `yaml:"verbose"`
}

// TerraformState represents the structure of the terraform show -json output
type TerraformState struct {
	Values struct {
		RootModule struct {
			Resources []struct {
				Address string                 `json:"address"`
				Values  map[string]interface{} `json:"values"`
			} `json:"resources"`
		} `json:"root_module"`
	} `json:"values"`
}

// ProviderSchema represents the structure of the provider schema JSON output.
type ProviderSchema struct {
	ProviderSchemas map[string]ProviderSchemaDetails `json:"provider_schemas"`
}

// ProviderSchemaDetails represents the details of a provider schema.
type ProviderSchemaDetails struct {
	ResourceSchemas   map[string]ResourceSchema `json:"resource_schemas"`
	DataSourceSchemas map[string]ResourceSchema `json:"data_source_schemas"`
}

// ResourceSchema represents the schema of a resource.
type ResourceSchema struct {
	Block ResourceBlock `json:"block"`
}

// ResourceBlock represents the block of a resource schema.
type ResourceBlock struct {
	Attributes map[string]Attribute `json:"attributes"`
}

// Attribute represents an attribute in a resource block.
type Attribute struct {
	Type        interface{} `json:"type"`
	Description string      `json:"description"`
	Optional    bool        `json:"optional"`
	Computed    bool        `json:"computed"`
	Required    bool        `json:"required"`
}

// AnnotatedResource represents an annotated resource found in a Terraform file
type AnnotatedResource struct {
	File     string
	Line     int
	Resource string
	Provider string
}

// Function to check for @interface annotations and extract resource type and name
func findAnnotatedResources(path string, verbose bool) []AnnotatedResource {
	var annotatedResources []AnnotatedResource

	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(info.Name(), ".tf") || strings.HasSuffix(info.Name(), ".tf.json")) {
			if verbose {
				log.Printf("Processing file: %s", path)
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			inAnnotation := false
			resourceBlock := ""
			lineNumber := 0
			for scanner.Scan() {
				lineNumber++
				line := strings.TrimSpace(scanner.Text())
				if verbose {
					log.Printf("Reading line: %s", line)
				}
				if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
					if strings.Contains(line, "@interface") {
						inAnnotation = true
						if verbose {
							log.Println("@interface annotation found")
						}
					}
				} else if inAnnotation {
					if strings.HasPrefix(line, "resource") {
						resourceBlock = line
						for scanner.Scan() {
							lineNumber++
							line := strings.TrimSpace(scanner.Text())
							resourceBlock += "\n" + line
							if strings.HasPrefix(line, "}") {
								break
							}
						}
						resourceInfo, provider := extractResourceInfo(resourceBlock)
						if resourceInfo != "" {
							annotatedResources = append(annotatedResources, AnnotatedResource{
								File:     path,
								Line:     lineNumber,
								Resource: resourceInfo,
								Provider: provider,
							})
						}
						resourceBlock = ""
						inAnnotation = false
					}
				}
			}
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Failed to walk through the Terraform project path: %v", err)
	}

	return annotatedResources
}

// Function to extract resource type, name and provider
func extractResourceInfo(resourceBlock string) (string, string) {
	re := regexp.MustCompile(`resource\s+"(\w+)"\s+"(\w+)"`)
	matches := re.FindStringSubmatch(resourceBlock)
	if len(matches) == 3 {
		resourceType := matches[1]
		resourceName := matches[2]

		// Extract provider
		reProvider := regexp.MustCompile(`provider\s*=\s*"([^"]+)"`)
		providerMatch := reProvider.FindStringSubmatch(resourceBlock)
		var provider string
		if len(providerMatch) == 2 {
			provider = providerMatch[1]
		}

		return fmt.Sprintf("%s.%s", resourceType, resourceName), provider
	}
	return "", ""
}

// Function to fetch the JSON state of the Terraform resources
func fetchTerraformState(shell string, command string, projectPath string, verbose bool) TerraformState {
	cmdStr := fmt.Sprintf(`%s show -json`, command)
	if verbose {
		log.Printf("Running command: %s -c \"%s\"", shell, cmdStr)
	}
	cmd := exec.Command(shell, "-c", cmdStr)
	cmd.Dir = projectPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to run command: %v, output: %s", err, output)
	}

	var state TerraformState
	if err := json.Unmarshal(output, &state); err != nil {
		log.Fatalf("Failed to parse JSON output: %v", err)
	}

	return state
}

// Function to fetch the provider schema JSON
func fetchProviderSchema(shell string, command string, projectPath string, verbose bool) ProviderSchema {
	cmdStr := fmt.Sprintf(`%s providers schema -json`, command)
	if verbose {
		log.Printf("Running command: %s -c \"%s\"", shell, cmdStr)
	}
	cmd := exec.Command(shell, "-c", cmdStr)
	cmd.Dir = projectPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to run command: %v, output: %s", err, output)
	}

	var schema ProviderSchema
	if err := json.Unmarshal(output, &schema); err != nil {
		log.Fatalf("Failed to parse JSON output: %v", err)
	}

	return schema
}

// Function to find required attributes for a given resource type from the schema
func findRequiredAttributes(schema ProviderSchema, resourceType string) []string {
	var requiredAttributes []string
	for _, providerSchema := range schema.ProviderSchemas {
		if resourceSchema, exists := providerSchema.ResourceSchemas[resourceType]; exists {
			for attributeName, attribute := range resourceSchema.Block.Attributes {
				if attribute.Required {
					requiredAttributes = append(requiredAttributes, attributeName)
				}
			}
		}
	}
	return requiredAttributes
}

// Function to create the "interface" directory if it doesn't exist
func createInterfaceDirectory(path string) string {
	interfaceDir := filepath.Join(path, "interface")
	if _, err := os.Stat(interfaceDir); os.IsNotExist(err) {
		err := os.Mkdir(interfaceDir, 0755)
		if err != nil {
			log.Fatalf("Failed to create directory %s: %v", interfaceDir, err)
		}
		log.Printf("Created directory: %s", interfaceDir)
	} else {
		log.Printf("Directory already exists: %s", interfaceDir)
	}
	return interfaceDir
}

// Function to create a Terraform file with data resources
func createTerraformFile(interfaceDir string, resources []AnnotatedResource, state TerraformState, schema ProviderSchema) {
	filePath := filepath.Join(interfaceDir, "generated_data.tf")
	file, err := os.Create(filePath)
	if err != nil {
		log.Fatalf("Failed to create Terraform file %s: %v", filePath, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	for _, resource := range resources {
		parts := strings.Split(resource.Resource, ".")
		if len(parts) != 2 {
			log.Fatalf("Invalid resource format: %s", resource.Resource)
		}
		resourceType := parts[0]
		resourceName := parts[1]

		hasMatchingDataResource, dataResourceRequiredAttributes := findMatchingDataResource(resource.Resource, schema)
		if hasMatchingDataResource {
			fmt.Fprintf(writer, "data \"%s\" \"%s\" {\n", resourceType, resourceName)
			for _, attr := range dataResourceRequiredAttributes {
				if value, exists := extractAttributeValue(resource.Resource, attr, state); exists {
					fmt.Fprintf(writer, "  %s = \"%v\"\n", attr, value)
				}
			}
			fmt.Fprintf(writer, "}\n\n")
		}
	}

	writer.Flush()
	log.Printf("Created Terraform file: %s", filePath)
}

// Function to check if a matching data resource exists in the schema
func findMatchingDataResource(resource string, schema ProviderSchema) (bool, []string) {
	parts := strings.Split(resource, ".")
	if len(parts) != 2 {
		log.Fatalf("Invalid resource format: %s", resource)
	}
	resourceType := parts[0]

	for _, providerSchema := range schema.ProviderSchemas {
		if dataSourceSchema, exists := providerSchema.DataSourceSchemas[resourceType]; exists {
			var requiredAttributes []string
			for attributeName, attribute := range dataSourceSchema.Block.Attributes {
				if attribute.Required {
					requiredAttributes = append(requiredAttributes, attributeName)
				}
			}
			return true, requiredAttributes
		}
	}
	return false, nil
}

// Function to extract an attribute value from the state
func extractAttributeValue(resource string, attribute string, state TerraformState) (interface{}, bool) {
	for _, res := range state.Values.RootModule.Resources {
		if res.Address == resource {
			if value, exists := res.Values[attribute]; exists {
				return value, true
			}
		}
	}
	return nil, false
}

// Function to create a Terraform file with provider blocks
func createProviderFile(interfaceDir string, resources []AnnotatedResource, schema ProviderSchema) {
	filePath := filepath.Join(interfaceDir, "generated_providers.tf")
	file, err := os.Create(filePath)
	if err != nil {
		log.Fatalf("Failed to create Terraform file %s: %v", filePath, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	providers := make(map[string]string)

	for _, resource := range resources {
		parts := strings.Split(resource.Resource, ".")
		if len(parts) != 2 {
			log.Fatalf("Invalid resource format: %s", resource.Resource)
		}
		resourceType := parts[0]

		for providerSource, providerSchema := range schema.ProviderSchemas {
			if _, exists := providerSchema.ResourceSchemas[resourceType]; exists {
				providerName := strings.Split(providerSource, "/")[2]
				providers[providerName] = providerSource
			}
		}
	}

	fmt.Fprintln(writer, "terraform {")
	fmt.Fprintln(writer, "  required_providers {")
	for provider, source := range providers {
		fmt.Fprintf(writer, "    %s = {\n", provider)
		fmt.Fprintf(writer, "      source = \"%s\"\n", source)
		fmt.Fprintln(writer, "    }")
	}
	fmt.Fprintln(writer, "  }")
	fmt.Fprintln(writer, "}")

	writer.Flush()
	log.Printf("Created Terraform provider file: %s", filePath)
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
		log.Println("Config file found and read successfully.")
		if err := yaml.Unmarshal(configFile, &config); err != nil {
			log.Fatalf("Failed to parse config file: %v", err)
		}
	} else {
		log.Printf("Config file not found or failed to read: %v", err)
	}

	// Output config file content for debugging
	if *verboseFlag {
		log.Printf("Config file content: %s", string(configFile))
	}
	log.Printf("Parsed config: %+v", config)

	// Determine which shell to use
	shell := "bash"
	if *shellFlag != "" {
		shell = *shellFlag
	} else if config.Shell != "" {
		shell = config.Shell
	}

	// Check if shell exists and is executable
	shellPath, err := exec.LookPath(shell)
	if err != nil {
		log.Fatalf("Shell not found or not executable: %s", shell)
	}
	log.Printf("Using shell path: %s", shellPath)

	// Determine the paths to the Terraform projects
	var terraformProjectPaths []string
	if *projectPathFlag != "" {
		terraformProjectPaths = []string{*projectPathFlag}
	} else if len(config.TerraformProjectPaths) > 0 {
		terraformProjectPaths = config.TerraformProjectPaths
	} else {
		terraformProjectPaths = []string{"."}
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
	log.Printf("Terraform project paths:\n%s", strings.Join(terraformProjectPaths, "\n"))
	log.Printf("Using tofu: %v", useTofu)
	log.Printf("Verbose output: %v", verbose)

	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}

	// Ensure PATH is inherited
	envPath := os.Getenv("PATH")
	if verbose {
		log.Printf("Environment PATH: %s", envPath)
	}

	// Iterate over each Terraform project path
	for _, terraformProjectPath := range terraformProjectPaths {
		fullPath := filepath.Join(currentDir, terraformProjectPath)
		// Change directory to the Terraform project path
		log.Printf("Changing directory to Terraform project: %s", fullPath)
		if err := os.Chdir(fullPath); err != nil {
			log.Fatalf("Failed to change directory to Terraform project: %v", err)
		}

		// Fetch the Terraform state
		command := "terraform"
		if useTofu {
			command = "tofu"
		}
		state := fetchTerraformState(shell, command, fullPath, verbose)

		// Fetch the provider schema
		schema := fetchProviderSchema(shell, command, fullPath, verbose)

		// Find and print annotated resources
		annotatedResources := findAnnotatedResources(".", verbose)
		if len(annotatedResources) == 0 {
			log.Println("No Annotated Resources")
		} else {
			log.Println("Annotated Resources:")
			var validResources []AnnotatedResource
			for _, resource := range annotatedResources {
				hasMatchingDataResource, _ := findMatchingDataResource(resource.Resource, schema)
				if hasMatchingDataResource {
					validResources = append(validResources, resource)
				} else {
					fmt.Printf("\033[31mAnnotated resource %s at line %d in file %s does not have a matching data resource!\033[0m\n", resource.Resource, resource.Line, resource.File)
				}
			}
			if len(validResources) > 0 {
				interfaceDir := createInterfaceDirectory(fullPath)
				createTerraformFile(interfaceDir, validResources, state, schema)
				createProviderFile(interfaceDir, validResources, schema)
			}
		}

		// Return to the original directory before moving to the next project path
		if err := os.Chdir(currentDir); err != nil {
			log.Fatalf("Failed to return to the original directory: %v", err)
		}
	}
}
