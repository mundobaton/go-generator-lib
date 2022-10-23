package acceptance

import (
	"context"
	generatorlib "github.com/mundobaton/go-generator-lib"
	"github.com/mundobaton/go-generator-lib/api"
	"github.com/mundobaton/go-generator-lib/docs"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestObtainGeneratorSpec_ShouldReturnCorrectSpec(t *testing.T) {
	docs.Given("a valid generator source directory")
	sourcedir := "../resources/valid-generator-simple"

	docs.Given("a valid generator name")
	name := "docker"

	docs.When("ObtainGeneratorSpec is invoked")
	actual, err := generatorlib.ObtainGeneratorSpec(context.TODO(), sourcedir, name)

	docs.Then("the correct spec is returned")
	expected := &api.GeneratorSpec{
		Templates: []api.TemplateSpec{
			{
				RelativeSourcePath: "Dockerfile.tmpl",
				RelativeTargetPath: "Dockerfile",
				Condition:          "",
				WithItems:          nil,
			},
		},
		Variables: map[string]api.VariableSpec{
			"serviceName": {
				Description:       "The name of the service to be rendered",
				ValidationPattern: "[a-zA-Z]+",
			},
		},
	}
	require.Nil(t, err)
	require.Equal(t, expected, actual)
}

func TestObtainGeneratorSpec_ShouldReturnCorrectStructuredSpec(t *testing.T) {
	docs.Given("a valid generator source directory")
	sourcedir := "../resources/valid-generator-structured"

	docs.Given("a valid generator name")
	name := "main"

	docs.When("ObtainGeneratorSpec is invoked")
	actual, err := generatorlib.ObtainGeneratorSpec(context.TODO(), sourcedir, name)

	docs.Then("the correct spec is returned")
	expected := &api.GeneratorSpec{
		Templates: []api.TemplateSpec{
			{
				RelativeSourcePath: "main.txt.tmpl",
				RelativeTargetPath: "main.txt",
				Condition:          "",
				WithItems:          nil,
			},
		},
		Variables: map[string]api.VariableSpec{
			"helloMessage": {
				Description:  "A message to be inserted in the code.",
				DefaultValue: "hello world",
			},
			"structureList": {
				Description: "A structured parameter that is a list at top level",
				DefaultValue: []interface{}{
					"one",
					"two",
					map[interface{}]interface{}{
						"three": []interface{}{
							"sub 1",
							"sub 2",
						},
					},
				},
			},
			"structureMap": {
				Description: "A structured parameter that is a map at top level",
				DefaultValue: map[interface{}]interface{}{
					"species":    "felis silvestris",
					"commonName": "European wildcat",
				},
			},
		},
	}
	require.Nil(t, err)
	require.Equal(t, expected, actual)
}

func TestObtainGeneratorSpec_ShouldFailOnMissingGeneratorFile(t *testing.T) {
	docs.Given("a valid generator source directory")
	sourcedir := "../resources/valid-generator-simple"

	docs.Given("an invalid generator name")
	name := "notthere"

	docs.When("ObtainGeneratorSpec is invoked")
	actual, err := generatorlib.ObtainGeneratorSpec(context.TODO(), sourcedir, name)

	docs.Then("an appropriate error is returned")
	require.Equal(t, &api.GeneratorSpec{}, actual)
	require.NotNil(t, err)
	expectedErrPart := "error reading generator spec file generator-notthere.yaml: open ../resources/valid-generator-simple/generator-notthere.yaml: "
	require.Contains(t, err.Error(), expectedErrPart)
}

func TestObtainGeneratorSpec_ShouldFailOnInvalidGeneratorYaml(t *testing.T) {
	docs.Given("a valid generator source directory")
	sourcedir := "../resources/invalid-generator-specs"

	docs.Given("a valid generator name with a syntactically invalid spec")
	name := "duplicatekey"

	docs.When("ObtainGeneratorSpec is invoked")
	actual, err := generatorlib.ObtainGeneratorSpec(context.TODO(), sourcedir, name)

	docs.Then("an appropriate error is returned")
	require.Equal(t, &api.GeneratorSpec{}, actual)
	require.NotNil(t, err)
	expectedErr := "error parsing generator spec from file generator-duplicatekey.yaml: yaml: unmarshal errors:\n  line 11: key \"identical\" already set in map"
	require.Equal(t, expectedErr, err.Error())
}

func TestObtainGeneratorSpec_ShouldFailOnGeneratorYamlWithUnknownKeys(t *testing.T) {
	docs.Given("a valid generator source directory")
	sourcedir := "../resources/invalid-generator-specs"

	docs.Given("a valid generator name with a syntactically valid spec that contains an unknown key")
	name := "unknownkey"

	docs.When("ObtainGeneratorSpec is invoked")
	actual, err := generatorlib.ObtainGeneratorSpec(context.TODO(), sourcedir, name)

	docs.Then("an appropriate error is returned")
	require.Equal(t, &api.GeneratorSpec{}, actual)
	require.NotNil(t, err)
	expectedErr := "error parsing generator spec from file generator-unknownkey.yaml: yaml: unmarshal errors:\n  line 1: field notvalid not found in type api.GeneratorSpec"
	require.Equal(t, expectedErr, err.Error())
}

func TestObtainGeneratorSpec_ShouldFailOnGeneratorDirWithTrailingSlash(t *testing.T) {
	docs.Given("an invalid generator source directory")
	sourcedir := "../resources/invalid-generator-specs/"

	docs.When("ObtainGeneratorSpec is invoked")
	actual, err := generatorlib.ObtainGeneratorSpec(context.TODO(), sourcedir, "doesnotmatter")

	docs.Then("an appropriate error is returned")
	require.Equal(t, &api.GeneratorSpec{}, actual)
	require.NotNil(t, err)
	expectedErr := "invalid generator directory: baseDir ../resources/invalid-generator-specs/ must not contain trailing slash"
	require.Equal(t, expectedErr, err.Error())
}
