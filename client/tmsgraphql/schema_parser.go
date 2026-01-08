package tmsgraphql

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

type SchemaValidationMessages struct {
	messages map[string]string
}

func NewSchemaValidationMessages(schemaPath string) (*SchemaValidationMessages, error) {
	s := &SchemaValidationMessages{
		messages: make(map[string]string),
	}

	files, err := filepath.Glob(filepath.Join(schemaPath, "*.graphqls"))
	if err != nil {
		return nil, err
	}

	graphqlFiles, err := filepath.Glob(filepath.Join(schemaPath, "*.graphql"))
	if err == nil {
		files = append(files, graphqlFiles...)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no GraphQL schema files found in %s", schemaPath)
	}

	var sources []*ast.Source

	for _, file := range files {
		content, err := ioutil.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read schema file %s: %w", file, err)
		}
		sources = append(sources, &ast.Source{
			Name:  file,
			Input: string(content),
		})
	}

	schema, err := gqlparser.LoadSchema(sources...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL schema: %w", err)
	}

	for _, def := range schema.Types {
		if def.Kind == ast.InputObject {
			s.extractMessagesFromType(def)
		}
	}

	return s, nil
}

func (s *SchemaValidationMessages) extractMessagesFromType(typeDef *ast.Definition) {
	inputType := typeDef.Name

	for _, field := range typeDef.Fields {
		for _, directive := range field.Directives {
			if directive.Name == "validateMessage" {
				rule := ""
				message := ""

				for _, arg := range directive.Arguments {
					switch arg.Name {
					case "rule":
						rule = arg.Value.Raw
					case "message":
						message = arg.Value.Raw
					}
				}

				if rule != "" && message != "" {
					key := fmt.Sprintf("%s.%s.%s", inputType, field.Name, rule)
					s.messages[key] = message
				}
			}
		}
	}
}

func (s *SchemaValidationMessages) GetMessage(inputType, field, rule string) (string, bool) {
	if s == nil {
		return "", false
	}

	key := fmt.Sprintf("%s.%s.%s", inputType, field, rule)
	if msg, exists := s.messages[key]; exists {
		return msg, true
	}
	return "", false
}

var _ ValidationMessageStore = (*SchemaValidationMessages)(nil)

type ValidationMetadata struct {
	InputType string                     `json:"inputType"`
	Fields    map[string]FieldValidation `json:"fields"`
}

type FieldValidation struct {
	Field       string           `json:"field"`
	Type        string           `json:"type"`
	Required    bool             `json:"required"`
	Constraint  string           `json:"constraint,omitempty"`
	Description string           `json:"description,omitempty"`
	Rules       []ValidationRule `json:"rules"`
}

type ValidationRule struct {
	Rule    string `json:"rule"`
	Value   string `json:"value,omitempty"`
	Message string `json:"message"`
}

func ExtractValidationMetadata(schemaPath string) (map[string]ValidationMetadata, error) {
	metadata := make(map[string]ValidationMetadata)

	files, err := filepath.Glob(filepath.Join(schemaPath, "*.graphqls"))
	if err != nil {
		return nil, err
	}

	graphqlFiles, err := filepath.Glob(filepath.Join(schemaPath, "*.graphql"))
	if err == nil {
		files = append(files, graphqlFiles...)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no GraphQL schema files found in %s", schemaPath)
	}

	var sources []*ast.Source
	for _, file := range files {
		content, err := ioutil.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read schema file %s: %w", file, err)
		}
		sources = append(sources, &ast.Source{
			Name:  file,
			Input: string(content),
		})
	}

	schema, err := gqlparser.LoadSchema(sources...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL schema: %w", err)
	}

	for _, def := range schema.Types {
		if def.Kind == ast.InputObject {
			typeMeta := ValidationMetadata{
				InputType: def.Name,
				Fields:    make(map[string]FieldValidation),
			}

			for _, field := range def.Fields {
				fieldMeta := FieldValidation{
					Field:       field.Name,
					Type:        field.Type.String(),
					Required:    field.Type.NonNull,
					Description: field.Description,
					Rules:       make([]ValidationRule, 0),
				}

				// Извлекаем constraint из @validate
				var validateDirective *ast.Directive
				var validateMessageDirectives []*ast.Directive

				for _, directive := range field.Directives {
					if directive.Name == "validate" {
						validateDirective = directive
					} else if directive.Name == "validateMessage" {
						validateMessageDirectives = append(validateMessageDirectives, directive)
					}
				}

				if validateDirective != nil {
					for _, arg := range validateDirective.Arguments {
						if arg.Name == "constraint" {
							fieldMeta.Constraint = arg.Value.Raw
							// Парсим constraint в правила
							fieldMeta.Rules = parseConstraintToRules(arg.Value.Raw, validateMessageDirectives)
						}
					}
				}

				typeMeta.Fields[field.Name] = fieldMeta
			}

			metadata[def.Name] = typeMeta
		}
	}

	return metadata, nil
}

func parseConstraintToRules(constraint string, messageDirectives []*ast.Directive) []ValidationRule {
	rules := make([]ValidationRule, 0)

	parts := strings.Split(constraint, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		ruleParts := strings.SplitN(part, "=", 2)
		rule := ValidationRule{
			Rule: ruleParts[0],
		}

		if len(ruleParts) > 1 {
			rule.Value = ruleParts[1]
		}

		for _, directive := range messageDirectives {
			ruleArg := ""
			messageArg := ""

			for _, arg := range directive.Arguments {
				switch arg.Name {
				case "rule":
					ruleArg = arg.Value.Raw
				case "message":
					messageArg = arg.Value.Raw
				}
			}

			if ruleArg == rule.Rule && messageArg != "" {
				rule.Message = strings.ReplaceAll(messageArg, "{value}", rule.Value)
				break
			}
		}

		if rule.Message == "" {
			rule.Message = getDefaultMessageForRule(rule.Rule, rule.Value)
		}

		rules = append(rules, rule)
	}

	return rules
}

func getDefaultMessageForRule(rule string, value string) string {
	switch rule {
	case "required":
		return "This field is required"
	case "min":
		if value != "" {
			return fmt.Sprintf("Minimum value is %s", value)
		}
		return "Minimum value required"
	case "max":
		if value != "" {
			return fmt.Sprintf("Maximum value is %s", value)
		}
		return "Maximum value exceeded"
	case "len":
		if value != "" {
			return fmt.Sprintf("Length must be exactly %s", value)
		}
		return "Invalid length"
	case "email":
		return "Must be a valid email address"
	case "url":
		return "Must be a valid URL"
	case "alpha":
		return "Must contain only alphabetic characters"
	case "alphanum":
		return "Must contain only alphanumeric characters"
	case "numeric":
		return "Must contain only numeric characters"
	default:
		if value != "" {
			return fmt.Sprintf("%s: %s", rule, value)
		}
		return rule
	}
}
