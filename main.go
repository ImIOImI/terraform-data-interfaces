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

type Config struct {
	Shell                 string   `yaml:"shell"`
	TerraformProjectPaths []string `yaml:"terraform_project_paths"`
	UseTofu               bool     `yaml:"use_tofu"`
	Verbose               bool     `yaml:"verbose"`
}

type TerraformState struct {
	Values struct {
		RootModule struct {
			Resources []struct {
				Address string                 `json:"address"`
				Values  map[string]interface{} `json:"values"`
			} `json:"resources"`
			Outputs map[string]struct {
				Value interface{} `json:"value"`
			} `json:"outputs"`
		} `json:"root_module"`
	} `json:"values"`
}

type ProviderSchema struct {
	ProviderSchemas map[string]ProviderSchemaDetails `json:"provider_schemas"`
}

type ProviderSchemaDetails struct {
	ResourceSchemas   map[string]ResourceSchema `json:"resource_schemas"`
	DataSourceSchemas map[string]ResourceSchema `json:"data_source_schemas"`
}

type ResourceSchema struct {
	Block ResourceBlock `json:"block"`
}

type ResourceBlock struct {
	Attributes map[string]Attribute `json:"attributes"`
}

type Attribute struct {
	Type        interface{} `json:"type"`
	Description string      `json:"description"`
	Optional    bool        `json:"optional"`
	Computed    bool        `json:"computed"`
	Required    bool        `json:"required"`
}

type AnnotatedOutput struct {
	File      string
	Line      int
	Output    string
	Reference string
	Provider  string
}

func findAnnotatedOutputs(path string, verbose bool) []AnnotatedOutput {
	var annotatedOutputs []AnnotatedOutput
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
			outputBlock := ""
			lineNumber := 0
			for scanner.Scan() {
				lineNumber++
				line := strings.TrimSpace(scanner.Text())
				if verbose {
					log.Printf("Reading line: %s", line)
				}
				if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
					if strings.Contains(line, "@public") {
						inAnnotation = true
						if verbose {
							log.Println("@public annotation found")
						}
					}
				} else if inAnnotation {
					if strings.HasPrefix(line, "output") {
						outputBlock = line
						for scanner.Scan() {
							lineNumber++
							line := strings.TrimSpace(scanner.Text())
							outputBlock += "\n" + line
							if strings.HasPrefix(line, "}") {
								break
							}
						}
						outputInfo, reference := extractOutputInfo(outputBlock)
						if outputInfo != "" {
							annotatedOutputs = append(annotatedOutputs, AnnotatedOutput{
								File:      path,
								Line:      lineNumber,
								Output:    outputInfo,
								Reference: reference,
							})
						}
						outputBlock = ""
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
	return annotatedOutputs
}

func extractOutputInfo(outputBlock string) (string, string) {
	re := regexp.MustCompile(`output\s+"(\w+)"`)
	matches := re.FindStringSubmatch(outputBlock)
	if len(matches) == 2 {
		outputName := matches[1]
		reReference := regexp.MustCompile(`value\s*=\s*(.+)`)
		referenceMatch := reReference.FindStringSubmatch(outputBlock)
		var reference string
		if len(referenceMatch) == 2 {
			reference = referenceMatch[1]
		}
		return outputName, reference
	}
	return "", ""
}

func fetchTerraformState(shell string, command string, projectPath string, verbose bool) (TerraformState, error) {
	cmdStr := fmt.Sprintf(`%s show -json`, command)
	if verbose {
		log.Printf("Running command: %s -c \"%s\"", shell, cmdStr)
	}
	cmd := exec.Command(shell, "-c", cmdStr)
	cmd.Dir = projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return TerraformState{}, fmt.Errorf("failed to run command: %v, output: %s", err, output)
	}
	var state TerraformState
	if err := json.Unmarshal(output, &state); err != nil {
		return TerraformState{}, fmt.Errorf("failed to parse JSON output: %v", err)
	}
	return state, nil
}

func fetchProviderSchema(shell string, command string, projectPath string, verbose bool) (ProviderSchema, error) {
	cmdStr := fmt.Sprintf(`%s providers schema -json`, command)
	if verbose {
		log.Printf("Running command: %s -c \"%s\"", shell, cmdStr)
	}
	cmd := exec.Command(shell, "-c", cmdStr)
	cmd.Dir = projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ProviderSchema{}, fmt.Errorf("failed to run command: %v, output: %s", err, output)
	}
	var schema ProviderSchema
	if err := json.Unmarshal(output, &schema); err != nil {
		return ProviderSchema{}, fmt.Errorf("failed to parse JSON output: %v", err)
	}
	return schema, nil
}

func findMatchingDataResource(reference string, schema ProviderSchema) (bool, []string) {
	parts := strings.Split(reference, ".")
	if len(parts) < 2 {
		log.Fatalf("Invalid reference format: %s", reference)
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

func extractAttributeValue(reference string, attribute string, state TerraformState) (interface{}, bool) {
	for _, res := range state.Values.RootModule.Resources {
		if res.Address == reference {
			if value, exists := res.Values[attribute]; exists {
				return value, true
			}
		}
	}
	return nil, false
}

func getResourceState(reference string, state TerraformState) (map[string]interface{}, bool) {
	for _, res := range state.Values.RootModule.Resources {
		if res.Address == reference {
			return res.Values, true
		}
	}
	return nil, false
}

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

func createTerraformFile(interfaceDir string, outputs []AnnotatedOutput, state TerraformState, schema ProviderSchema) {
	filePath := filepath.Join(interfaceDir, "generated_data.tf")
	file, err := os.Create(filePath)
	if err != nil {
		log.Fatalf("Failed to create Terraform file %s: %v", filePath, err)
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	seenResources := make(map[string]bool)
	for _, output := range outputs {
		parts := strings.Split(output.Reference, ".")
		if len(parts) != 3 {
			log.Fatalf("Invalid reference format: %s", output.Reference)
		}
		resourceType := parts[0]
		resourceName := parts[1]
		uniqueKey := fmt.Sprintf("%s.%s", resourceType, resourceName)
		if _, seen := seenResources[uniqueKey]; !seen {
			seenResources[uniqueKey] = true
			// Remove the attribute from the reference
			resourceReference := fmt.Sprintf("%s.%s", resourceType, resourceName)
			fmt.Printf("\033[32mLooking up resource: %s\033[0m\n", resourceReference)
			hasMatchingDataResource, dataResourceRequiredAttributes := findMatchingDataResource(resourceReference, schema)
			if hasMatchingDataResource {
				resourceState, exists := getResourceState(resourceReference, state)
				if exists {
					fmt.Printf("\033[32mMatching resource: %s\033[0m\n", resourceReference)
					fmt.Printf("\033[32mResource State for %s:\n%+v\033[0m\n", resourceReference, resourceState)
					// Log the full state
					stateJSON, _ := json.MarshalIndent(resourceState, "", "  ")
					fmt.Printf("\033[32mFull Resource State for %s:\n%s\033[0m\n", resourceReference, string(stateJSON))
				} else {
					log.Printf("Resource %s not found in state\n", resourceReference)
				}
				fmt.Printf("\033[32mData source: %s.%s\033[0m\n", resourceType, resourceName)
				fmt.Fprintf(writer, "data \"%s\" \"%s\" {\n", resourceType, resourceName)
				for _, attr := range dataResourceRequiredAttributes {
					if value, exists := extractAttributeValue(resourceReference, attr, state); exists {
						fmt.Fprintf(writer, "  %s = \"%v\"\n", attr, value)
						fmt.Printf("\033[32mRequired attribute: %s = %v\033[0m\n", attr, value)
					} else {
						fmt.Fprintf(writer, "  %s = \"\"\n", attr) // Default value if not found in state
						fmt.Printf("\033[32mRequired attribute: %s = <nil>\033[0m\n", attr)
					}
				}
				fmt.Fprintf(writer, "}\n\n")
			}
		}
	}
	writer.Flush()
	log.Printf("Created Terraform file: %s", filePath)
}

func createProviderFile(interfaceDir string, outputs []AnnotatedOutput, schema ProviderSchema) {
	filePath := filepath.Join(interfaceDir, "generated_providers.tf")
	file, err := os.Create(filePath)
	if err != nil {
		log.Fatalf("Failed to create Terraform file %s: %v", filePath, err)
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	providers := make(map[string]string)
	for _, output := range outputs {
		parts := strings.Split(output.Reference, ".")
		if len(parts) != 3 {
			log.Fatalf("Invalid reference format: %s", output.Reference)
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
	log.Printf("Created provider file: %s", filePath)
}

func main() {
	shellFlag := flag.String("shell", "", "Shell to use for executing commands")
	projectPathFlag := flag.String("project-path", "", "Path to the Terraform project")
	useTofuFlag := flag.Bool("use-tofu", false, "Use tofu instead of terraform for commands")
	verboseFlag := flag.Bool("verbose", false, "Enable verbose output")
	flag.Parse()
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
	if *verboseFlag {
		log.Printf("Config file content: %s", string(configFile))
	}
	log.Printf("Parsed config: %+v", config)
	shell := "bash"
	if *shellFlag != "" {
		shell = *shellFlag
	} else if config.Shell != "" {
		shell = config.Shell
	}
	shellPath, err := exec.LookPath(shell)
	if err != nil {
		log.Fatalf("Shell not found or not executable: %s", shell)
	}
	log.Printf("Using shell path: %s", shellPath)
	terraformProjectPaths := []string{"."}
	if *projectPathFlag != "" {
		terraformProjectPaths = []string{*projectPathFlag}
	} else if len(config.TerraformProjectPaths) > 0 {
		terraformProjectPaths = config.TerraformProjectPaths
	}
	useTofu := *useTofuFlag || config.UseTofu
	verbose := *verboseFlag || config.Verbose
	log.Printf("Using shell: %s", shell)
	log.Printf("Terraform project paths:\n%s", strings.Join(terraformProjectPaths, "\n"))
	log.Printf("Using tofu: %v", useTofu)
	log.Printf("Verbose output: %v", verbose)
	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}
	envPath := os.Getenv("PATH")
	if verbose {
		log.Printf("Environment PATH: %s", envPath)
	}
	for _, terraformProjectPath := range terraformProjectPaths {
		fullPath := filepath.Join(currentDir, terraformProjectPath)
		log.Printf("Changing directory to Terraform project: %s", fullPath)
		if err := os.Chdir(fullPath); err != nil {
			log.Fatalf("Failed to change directory to Terraform project: %v", err)
		}
		command := "terraform"
		if useTofu {
			command = "tofu"
		}
		state, err := fetchTerraformState(shell, command, fullPath, verbose)
		if err != nil {
			log.Fatalf("Failed to fetch Terraform state: %v", err)
		}
		schema, err := fetchProviderSchema(shell, command, fullPath, verbose)
		if err != nil {
			log.Fatalf("Failed to fetch provider schema: %v", err)
		}
		annotatedOutputs := findAnnotatedOutputs(".", verbose)
		if len(annotatedOutputs) == 0 {
			log.Println("No Annotated Outputs")
		} else {
			log.Println("Annotated Outputs:")
			var validOutputs []AnnotatedOutput
			for _, output := range annotatedOutputs {
				hasMatchingDataResource, _ := findMatchingDataResource(output.Reference, schema)
				if hasMatchingDataResource {
					validOutputs = append(validOutputs, output)
				} else {
					fmt.Printf("\033[31mAnnotated output %s at line %d in file %s does not have a matching data resource!\033[0m\n", output.Output, output.Line, output.File)
				}
			}
			if len(validOutputs) > 0 {
				interfaceDir := createInterfaceDirectory(fullPath)
				createTerraformFile(interfaceDir, validOutputs, state, schema)
				createProviderFile(interfaceDir, validOutputs, schema)
			}
		}
		if err := os.Chdir(currentDir); err != nil {
			log.Fatalf("Failed to return to the original directory: %v", err)
		}
	}
}
