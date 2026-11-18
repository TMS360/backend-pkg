package graphql

import (
	"io/fs"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

const minDescriptionLen = 10

const federationStub = `directive @shareable repeatable on OBJECT | FIELD_DEFINITION`

func shippedSchemas(t *testing.T) []*ast.Source {
	t.Helper()

	names, err := fs.Glob(SchemaFiles, "filters/*.graphqls")
	require.NoError(t, err)
	require.NotEmpty(t, names, "no schema files embedded")

	sources := make([]*ast.Source, 0, len(names))
	for _, name := range names {
		body, err := SchemaFiles.ReadFile(name)
		require.NoError(t, err)
		sources = append(sources, &ast.Source{Name: name, Input: string(body)})
	}

	return sources
}

func TestSharedSchemasLoadAsOneSchema(t *testing.T) {
	sources := append([]*ast.Source{{Name: "federation.graphqls", Input: federationStub, BuiltIn: true}}, shippedSchemas(t)...)

	_, err := gqlparser.LoadSchema(sources...)
	require.Nil(t, err, "shared schema files must load together: %v", err)
}

func TestSharedSchemasAreDescribed(t *testing.T) {
	checked := 0

	for _, src := range shippedSchemas(t) {
		doc, err := parser.ParseSchema(src)
		require.Nil(t, err, "%s must parse: %v", src.Name, err)

		for _, def := range doc.Definitions {
			checked++
			assertDescribes(t, src.Name, def.Name, def.Name, def.Description)

			for _, field := range def.Fields {
				path := def.Name + "." + field.Name
				assertDescribes(t, src.Name, path, field.Name, field.Description)

				for _, arg := range field.Arguments {
					assertDescribes(t, src.Name, path+"("+arg.Name+")", arg.Name, arg.Description)
				}
			}

			for _, value := range def.EnumValues {
				assertDescribes(t, src.Name, def.Name+"."+value.Name, value.Name, value.Description)
			}
		}

		require.Empty(t, doc.Extensions, "%s: a type extension cannot carry a description; declare the type instead", src.Name)
	}

	require.NotZero(t, checked, "no definitions found in the shared schema files")
}

func assertDescribes(t *testing.T, file, path, name, description string) {
	t.Helper()

	description = strings.TrimSpace(description)
	if !assert.NotEmpty(t, description, "%s: %s has no description", file, path) {
		return
	}

	assert.GreaterOrEqual(t, len(description), minDescriptionLen,
		"%s: %s description is too short to explain anything: %q", file, path, description)
	assert.NotEqual(t, normalize(name), normalize(description),
		"%s: %s description only repeats the name", file, path)
}

func normalize(s string) string {
	var b strings.Builder

	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}

	return b.String()
}
