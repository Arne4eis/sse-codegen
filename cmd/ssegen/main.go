package main

import (
	"embed"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/iancoleman/strcase"
	"gopkg.in/yaml.v3"
)

//go:embed templates/*.tmpl
var embeddedTemplates embed.FS

type (
	Config struct {
		Input    string
		Output   string
		TypeName string
		Package  string
		Lang     string
	}

	SSEEvent struct {
		Key         string `yaml:"key"`
		Event       string `yaml:"event"`
		Description string `yaml:"description,omitempty"`
		Deprecated  bool   `yaml:"deprecated,omitempty"`
	}

	OpenAPISpec struct {
		Components struct {
			SSEEvents map[string]SSEEvent `yaml:"x-sse-events"`
		} `yaml:"components"`
	}

	TemplateData struct {
		Package  string
		TypeName string
		Events   []SSEEvent
	}

	Lang string
)

const (
	Go string = "go"
	TS string = "ts"
)

func main() {
	config := parseFlags()

	if err := generateSSEEvents(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated %s enum in %s\n", config.Lang, config.Output)
}

func parseFlags() Config {
	config := &Config{
		TypeName: "SSEEvent",
		Package:  "events",
	}

	flag.StringVar(&config.Input, "i", "", "Input OpenAPI YAML file (required)")
	flag.StringVar(&config.Output, "o", "", "Output file path (required)")
	flag.StringVar(&config.Lang, "lang", "", "Target language (go, ts) (required)")
	flag.StringVar(&config.TypeName, "type", "SSEEvent", "Type name for enum")
	flag.StringVar(&config.Package, "package", "events", "Package name")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if config.Input == "" || config.Output == "" || config.Lang == "" {
		flag.Usage()
		os.Exit(1)
	}

	if config.Lang != string(Go) && config.Lang != string(TS) {
		flag.Usage()
		os.Exit(1)
	}

	return *config
}

func getEvents(inputFile string) ([]SSEEvent, error) {
	data, err := os.ReadFile(inputFile)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	var spec OpenAPISpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("error parsing YAML: %w", err)
	}

	var events []SSEEvent
	for key, event := range spec.Components.SSEEvents {
		event.Key = strcase.ToCamel(key)
		event.Event = strcase.ToLowerCamel(event.Event)
		events = append(events, event)
	}

	slices.SortFunc(events, func(a, b SSEEvent) int {
		return strings.Compare(a.Key, b.Key)
	})

	return events, nil
}

func generateSSEEvents(config Config) error {
	events, err := getEvents(config.Input)
	if err != nil {
		return fmt.Errorf("error getting events: %v", err)
	}

	tmplData := TemplateData{
		Package:  config.Package,
		TypeName: config.TypeName,
		Events:   events,
	}

	generated, err := generateCode(tmplData, config)
	if err != nil {
		return fmt.Errorf("error generating code: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(config.Output), 0755); err != nil {
		return fmt.Errorf("error creating directory: %v", err)
	}

	if err := os.WriteFile(config.Output, generated, 0644); err != nil {
		return fmt.Errorf("error writing file: %v", err)
	}

	return nil
}

func generateCode(tmplData TemplateData, config Config) ([]byte, error) {
	var generated strings.Builder

	switch config.Lang {
	case Go:
		tmplContent, err := embeddedTemplates.ReadFile("templates/go.tmpl")
		if err != nil {
			return nil, fmt.Errorf("error reading template: %v", err)
		}

		tmpl, err := template.New("go.tmpl").Parse(string(tmplContent))
		if err != nil {
			return nil, fmt.Errorf("error parsing template: %v", err)
		}

		if err := tmpl.Execute(&generated, tmplData); err != nil {
			return nil, fmt.Errorf("error executing template: %v", err)
		}

		return format.Source([]byte(generated.String()))

	case TS:
		tmplContent, err := embeddedTemplates.ReadFile("templates/ts.tmpl")
		if err != nil {
			return nil, fmt.Errorf("error reading template: %v", err)
		}

		tmpl, err := template.New("ts.tmpl").Parse(string(tmplContent))
		if err != nil {
			return nil, fmt.Errorf("error parsing template: %v", err)
		}

		if err := tmpl.Execute(&generated, tmplData); err != nil {
			return nil, fmt.Errorf("error executing template: %v", err)
		}

		return []byte(generated.String()), nil

	default:
		return nil, fmt.Errorf("unsupported language: %s", config.Lang)
	}
}
