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
func findAnnotatedResources(path string, variables map[string]string, verbose bool) []string {
	var annotatedResources []string

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
			for scanner.Scan() {
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
							line := strings.TrimSpace(scanner.Text())
							resourceBlock += "\n" + line
							if strings.HasPrefix(line, "}") {
								break
							}
						}
						resourceInfo := extractResourceInfo(resourceBlock, variables, path)
						if resourceInfo != "" {
							annotatedResources = append(annotatedResources, resourceInfo)
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

// Function to extract resource type and name
func extractResourceInfo(resourceBlock string, variables map[string]string, projectPath string) string {
	re := regexp.MustCompile(`resource\s+"(\w+)"\s+"(\w+)"`)
	matches := re.FindStringSubmatch(resourceBlock)
	if len(matches) == 3 {
		resourceType := matches[1]
		resourceName := matches[2]
		return fmt.Sprintf("Resource Type: %s, Name: %s\n%s", resourceType, resourceName, extractResourceAttributes(resourceBlock, variables, projectPath))
	}
	return ""
}

// Function to extract resource attributes with variable expansion
func extractResourceAttributes(resourceBlock string, variables map[string]string, projectPath string) string {
	attributes := ""
	re := regexp.MustCompile(`(\w+)\s+=\s+(.+)`)
	matches := re.FindAllStringSubmatch(resourceBlock, -1)
	for _, match := range matches {
		if len(match) == 3 {
			value := resolveTerraformValue(match[2], projectPath, variables["shell"], variables["command"], variables["verbose"] == "true")
			attributes += fmt.Sprintf("  %s: %s\n", match[1], value)
		}
	}
	return attributes
}

// Function to resolve a Terraform value using `terraform console`
func resolveTerraformValue(value string, projectPath string, shell string, command string, verbose bool) string {
	// First, test if the shell command works by running a simple echo command
	testCmd := exec.Command(shell, "-c", "echo test")
	testOutput, err := testCmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to run test command: %v, output: %s", err, testOutput)
	}
	if verbose {
		log.Printf("Test command output: %s", testOutput)
	}

	// Now, run the terraform console command
	consoleCommand := fmt.Sprintf("%s console", command)
	if verbose {
		log.Printf("Running command: %s -c \"%s\"", shell, consoleCommand)
	}
	cmd := exec.Command(shell, "-c", fmt.Sprintf("\"%s\"", consoleCommand))
	cmd.Dir = projectPath

	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatalf("Failed to get stdin pipe for %s console: %v", command, err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout pipe for %s console: %v", command, err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start %s console: %v", command, err)
	}

	if verbose {
		log.Printf("Writing to console: %s", value)
	}
	_, err = stdin.Write([]byte(value + "\n"))
	if err != nil {
		log.Fatalf("Failed to write to %s console: %v", command, err)
	}

	var result string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			result = line
			break
		}
	}

	if err := cmd.Wait(); err != nil {
		log.Fatalf("Failed to wait for %s console to finish: %v", command, err)
	}

	return result
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

	// Define known variables
	variables := map[string]string{
		"${path.module}": currentDir,
		"shell":          shell,
		"command":        "terraform",
		"verbose":        fmt.Sprintf("%v", verbose),
	}

	if useTofu {
		variables["command"] = "tofu"
	}

	// Ensure PATH is inherited
	envPath := os.Getenv("PATH")
	if verbose {
		log.Printf("Environment PATH: %s", envPath)
	}

	// Add tofu binary directory to PATH
	tofuDir := "/path/to/tofu" // Replace with the actual path to tofu
	envPath = fmt.Sprintf("%s:%s", tofuDir, envPath)
	if verbose {
		log.Printf("Updated PATH for tofu: %s", envPath)
	}

	// Iterate over each Terraform project path
	for _, terraformProjectPath := range terraformProjectPaths {
		fullPath := filepath.Join(currentDir, terraformProjectPath)
		// Change directory to the Terraform project path
		log.Printf("Changing directory to Terraform project: %s", fullPath)
		if err := os.Chdir(fullPath); err != nil {
			log.Fatalf("Failed to change directory to Terraform project: %v", err)
		}

		// Check if Terraform is applied
		if !isTerraformApplied(variables["shell"], variables["command"], envPath, verbose) {
			log.Printf("Terraform project at %s is not applied. Skipping.", fullPath)
			continue
		}

		// Find and print annotated resources
		annotatedResources := findAnnotatedResources(".", variables, verbose)
		if len(annotatedResources) == 0 {
			log.Println("No Annotated Resources")
		} else {
			log.Println("Annotated Resources:")
			for _, resource := range annotatedResources {
				fmt.Println(resource)
			}
		}

		// Determine the command to execute
		cmdStr := fmt.Sprintf("%s providers schema -json", variables["command"])
		if verbose {
			log.Printf("Running command: %s -c \"%s\"", shell, cmdStr)
		}
		cmd := exec.Command(shell, "-c", fmt.Sprintf("\"%s\"", cmdStr))
		cmd.Env = append(os.Environ(), fmt.Sprintf("PATH=%s", envPath)) // Ensure PATH is inherited

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

		// Return to the original directory before moving to the next project path
		if err := os.Chdir(currentDir); err != nil {
			log.Fatalf("Failed to return to the original directory: %v", err)
		}
	}
}

// Function to check if Terraform is applied
func isTerraformApplied(shell string, command string, envPath string, verbose bool) bool {
	cmdStr := fmt.Sprintf("%s show -json", command)
	if verbose {
		log.Printf("Running command: %s -c \"%s\"", shell, cmdStr)
	}
	cmd := exec.Command(shell, "-c", fmt.Sprintf("\"%s\"", cmdStr))
	cmd.Env = append(os.Environ(), fmt.Sprintf("PATH=%s", envPath)) // Ensure PATH is inherited
	output, err := cmd.CombinedOutput()
	if err != nil {
		if verbose {
			log.Printf("Command failed: %v", err)
			log.Printf("Command output: %s", output)
		}
		return false
	}
	if verbose {
		log.Printf("Command output: %s", output)
	}
	// Check if the output contains any resource state
	return strings.Contains(string(output), "resources")
}
