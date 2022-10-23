package implementation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/Masterminds/sprig"
	"github.com/mundobaton/go-generator-lib/api"
	"github.com/mundobaton/go-generator-lib/internal/implementation/templatewrapper"
	"github.com/mundobaton/go-generator-lib/internal/repository/generatordir"
	"github.com/mundobaton/go-generator-lib/internal/repository/targetdir"
	"regexp"
	"strings"
	"text/template"
)

type GeneratorImpl struct {
}

func (i *GeneratorImpl) FindGeneratorNames(ctx context.Context, sourceBaseDir string) ([]string, error) {
	sourceDir := generatordir.Instance(ctx, sourceBaseDir)
	return sourceDir.FindGeneratorNames(ctx)
}

func (i *GeneratorImpl) ObtainGeneratorSpec(ctx context.Context, sourceBaseDir string, generatorName string) (*api.GeneratorSpec, error) {
	sourceDir := generatordir.Instance(ctx, sourceBaseDir)
	return sourceDir.ObtainGeneratorSpec(ctx, generatorName)
}

func (i *GeneratorImpl) WriteRenderSpecWithDefaults(ctx context.Context, request *api.Request, generatorName string) *api.Response {
	sourceDir := generatordir.Instance(ctx, request.SourceBaseDir)
	targetDir := targetdir.Instance(ctx, request.TargetBaseDir)

	genSpec, err := sourceDir.ObtainGeneratorSpec(ctx, generatorName)
	if err != nil {
		return i.errorResponseToplevel(ctx, err)
	}

	// for missing default values, default to the empty string rather than nil
	// this makes the spec entry be an empty string, resulting in a valid render spec
	renderSpec, err := i.constructRenderSpecWithValuesOrDefaults(ctx, generatorName, genSpec, map[string]interface{}{}, "")
	if err != nil {
		return i.errorResponseToplevel(ctx, err)
	}

	// no validation here because the defaults may be empty or may intentionally not match the validation rule
	// (might be something like 'put in your fqdn name here')

	targetFile, err := targetDir.WriteRenderSpec(ctx, renderSpec, request.RenderSpecFile)
	if err != nil {
		return i.errorResponseToplevel(ctx, err)
	}
	return i.successResponse(ctx, []api.FileResult{i.successFileResult(ctx, targetFile)})
}

func (i *GeneratorImpl) WriteRenderSpecWithValues(ctx context.Context, request *api.Request, generatorName string, parameters map[string]interface{}) *api.Response {
	sourceDir := generatordir.Instance(ctx, request.SourceBaseDir)
	targetDir := targetdir.Instance(ctx, request.TargetBaseDir)

	genSpec, err := sourceDir.ObtainGeneratorSpec(ctx, generatorName)
	if err != nil {
		return i.errorResponseToplevel(ctx, err)
	}

	// when the user is providing a set of values for the parameter, we want missing parameter values to be reported as missing
	// therefore, actually set the nilDefault to nil
	renderSpec, err := i.constructRenderSpecWithValuesOrDefaults(ctx, generatorName, genSpec, parameters, nil)
	if err != nil {
		return i.errorResponseToplevel(ctx, err)
	}

	_, err = i.constructAndValidateParameterMap(ctx, genSpec, renderSpec)
	if err != nil {
		return i.errorResponseToplevel(ctx, err)
	}

	// check for extraneous parameters
	for k := range parameters {
		if _, ok := genSpec.Variables[k]; !ok {
			return i.errorResponseToplevel(ctx, fmt.Errorf("parameter '%s' is not allowed according to generator spec", k))
		}
	}

	targetFile, err := targetDir.WriteRenderSpec(ctx, renderSpec, request.RenderSpecFile)
	if err != nil {
		return i.errorResponseToplevel(ctx, err)
	}
	return i.successResponse(ctx, []api.FileResult{i.successFileResult(ctx, targetFile)})
}

func (i *GeneratorImpl) Render(ctx context.Context, request *api.Request) *api.Response {
	sourceDir := generatordir.Instance(ctx, request.SourceBaseDir)
	targetDir := targetdir.Instance(ctx, request.TargetBaseDir)

	renderSpec, err := targetDir.ObtainRenderSpec(ctx, request.RenderSpecFile)
	if err != nil {
		return i.errorResponseToplevel(ctx, err)
	}

	genSpec, err := sourceDir.ObtainGeneratorSpec(ctx, renderSpec.GeneratorName)
	if err != nil {
		return i.errorResponseToplevel(ctx, err)
	}

	parameters, err := i.constructAndValidateParameterMap(ctx, genSpec, renderSpec)
	if err != nil {
		return i.errorResponseToplevel(ctx, err)
	}

	renderedFiles, allSuccessful := i.renderAllTemplates(ctx, genSpec, parameters, sourceDir, targetDir)
	if allSuccessful {
		return i.successResponse(ctx, renderedFiles)
	} else {
		return i.errorResponseRender(ctx, renderedFiles)
	}
}

// helper functions

func (i *GeneratorImpl) constructRenderSpecWithValuesOrDefaults(_ context.Context, generatorName string, genSpec *api.GeneratorSpec, parameters map[string]interface{}, nilDefault interface{}) (*api.RenderSpec, error) {
	renderSpec := &api.RenderSpec{
		GeneratorName: generatorName,
		Parameters:    map[string]interface{}{},
	}
	for k, v := range genSpec.Variables {
		// a fetch on a map missing key will produce the empty value for that type, i.e. nil here
		renderSpec.Parameters[k] = parameters[k]
		if renderSpec.Parameters[k] == nil {
			if v.DefaultValue == nil {
				renderSpec.Parameters[k] = nilDefault
			} else if defaultStr, ok := v.DefaultValue.(string); ok {
				// again, the default may be the empty string
				renderedDefaultValue, err := i.renderStringDefaultFromTemplate(k, defaultStr)
				if err != nil {
					return nil, err
				}

				renderSpec.Parameters[k] = renderedDefaultValue
			} else {
				// structured type
				renderSpec.Parameters[k] = v.DefaultValue
			}
		}
	}
	return renderSpec, nil
}

func (i *GeneratorImpl) renderStringDefaultFromTemplate(variableName string, defaultStr string) (interface{}, error) {
	templateName := "__defaultvalue_" + variableName
	tmpl, err := template.New(templateName).Funcs(sprig.TxtFuncMap()).Parse(defaultStr)
	if err != nil {
		return nil, fmt.Errorf("variable declaration %s has invalid default (this is an error in the generator spec): %s", variableName, err.Error())
	}

	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, templateName, map[string]interface{}{})
	if err != nil {
		// unsure if this is reachable. All errors I've been able to produce are found during template parse
		return nil, fmt.Errorf("variable declaration %s has invalid default (this is an error in the generator spec): %s", variableName, err.Error())
	}

	return buf.String(), nil
}

func (i *GeneratorImpl) constructAndValidateParameterMap(_ context.Context, genSpec *api.GeneratorSpec, renderSpec *api.RenderSpec) (map[string]interface{}, error) {
	parameters := make(map[string]interface{})
	for varName, varSpec := range genSpec.Variables {
		val, ok := renderSpec.Parameters[varName]
		if !ok {
			if defaultStr, ok := varSpec.DefaultValue.(string); ok {
				renderedDefaultValue, err := i.renderStringDefaultFromTemplate(varName, defaultStr)
				if err != nil {
					return nil, err
				}

				val = renderedDefaultValue
			} else {
				val = varSpec.DefaultValue
			}
		}

		if val == nil {
			return nil, fmt.Errorf("parameter '%s' is required but missing", varName)
		}
		if varSpec.ValidationPattern != "" {
			matches, err := regexp.MatchString(varSpec.ValidationPattern, fmt.Sprintf("%v", val))
			if err != nil {
				return nil, fmt.Errorf("variable declaration %s has invalid pattern (this is an error in the generator spec, not the render request): %s", varName, err.Error())
			}
			if !matches {
				return nil, fmt.Errorf("value for parameter '%s' does not match pattern %s", varName, varSpec.ValidationPattern)
			}
		}
		parameters[varName] = val
	}
	return parameters, nil
}

func (i *GeneratorImpl) renderAllTemplates(ctx context.Context, genSpec *api.GeneratorSpec, parameters map[string]interface{}, sourceDir *generatordir.GeneratorDirectory, targetDir *targetdir.TargetDirectory) ([]api.FileResult, bool) {
	var renderedFiles []api.FileResult
	allSuccessful := true
	for _, tplSpec := range genSpec.Templates {
		rendered, success := i.renderSingleTemplate(ctx, &tplSpec, parameters, sourceDir, targetDir)
		renderedFiles = append(renderedFiles, rendered...)
		allSuccessful = allSuccessful && success
	}
	return renderedFiles, allSuccessful
}

func (i *GeneratorImpl) renderSingleTemplate(ctx context.Context, tplSpec *api.TemplateSpec, parameters map[string]interface{}, sourceDir *generatordir.GeneratorDirectory, targetDir *targetdir.TargetDirectory) ([]api.FileResult, bool) {
	templateName := strings.ReplaceAll(tplSpec.RelativeSourcePath, "/", "_")
	templateContents, err := sourceDir.ReadFile(ctx, tplSpec.RelativeSourcePath)
	if err != nil {
		return []api.FileResult{i.errorFileResult(ctx, tplSpec.RelativeTargetPath, fmt.Errorf("failed to load template %s: %s", tplSpec.RelativeSourcePath, err))}, false
	}

	tmplw, err := templatewrapper.New(tplSpec.JustCopy, templateContents, templateName, tplSpec.RelativeSourcePath).Parse()
	if err != nil {
		return []api.FileResult{i.errorFileResult(ctx, tplSpec.RelativeTargetPath, fmt.Errorf("failed to parse template %s: %s", tplSpec.RelativeSourcePath, err))}, false
	}

	renderedFiles := []api.FileResult{}
	allSuccessful := true
	if len(tplSpec.WithItems) > 0 {
		for counter, item := range tplSpec.WithItems {
			parameters["item"] = item
			renderedFiles, allSuccessful = i.renderSingleTemplateIteration(ctx, tplSpec, parameters, templateName, fmt.Sprintf("_%d", counter+1),
				fmt.Sprintf(" for item #%d", counter+1), renderedFiles, allSuccessful, tmplw, targetDir)
		}
	} else {
		renderedFiles, allSuccessful = i.renderSingleTemplateIteration(ctx, tplSpec, parameters, templateName, "",
			"", renderedFiles, allSuccessful, tmplw, targetDir)
	}
	return renderedFiles, allSuccessful
}

func (i *GeneratorImpl) renderSingleTemplateIteration(ctx context.Context, tplSpec *api.TemplateSpec, parameters map[string]interface{}, templateName string, templateNameExtension string,
	errorMessageItemExtension string, renderedFiles []api.FileResult, allSuccessful bool, tmpl *templatewrapper.TemplateWrapper, targetDir *targetdir.TargetDirectory) ([]api.FileResult, bool) {
	targetPath, err := i.renderString(ctx, parameters, fmt.Sprintf("%s_path%s", templateName, templateNameExtension), tplSpec.RelativeTargetPath)
	if err != nil {
		renderedFiles = append(renderedFiles, i.errorFileResult(ctx, targetPath, fmt.Errorf("error evaluating target path from '%s'%s: %s", tplSpec.RelativeTargetPath, errorMessageItemExtension, err)))
		allSuccessful = false
	} else {
		condition, err := i.evaluateCondition(ctx, tplSpec.Condition, parameters, fmt.Sprintf("%s_condition%s", templateName, templateNameExtension))
		if err != nil {
			renderedFiles = append(renderedFiles, i.errorFileResult(ctx, targetPath, fmt.Errorf("error evaluating condition from '%s'%s: %s", tplSpec.Condition, errorMessageItemExtension, err)))
			allSuccessful = false
		} else if condition {
			err := i.renderAndWriteFile(ctx, parameters, tmpl, templateName, targetDir, targetPath)
			if err != nil {
				renderedFiles = append(renderedFiles, i.errorFileResult(ctx, targetPath, fmt.Errorf("error evaluating template for target '%s'%s: %s", targetPath, errorMessageItemExtension, err)))
				allSuccessful = false
			} else {
				renderedFiles = append(renderedFiles, i.successFileResult(ctx, targetPath))
			}
		}
	}
	return renderedFiles, allSuccessful
}

func (i *GeneratorImpl) evaluateCondition(ctx context.Context, condition string, parameters map[string]interface{}, templateName string) (bool, error) {
	if condition == "" {
		return true, nil
	}
	rendered, err := i.renderString(ctx, parameters, templateName, condition)
	if err != nil {
		return false, err
	}
	return rendered != "false" && rendered != "0" && rendered != "no" && rendered != "skip", nil
}

func (i *GeneratorImpl) renderAndWriteFile(ctx context.Context, parameters map[string]interface{}, tmplw *templatewrapper.TemplateWrapper, templateName string, targetDir *targetdir.TargetDirectory, targetPath string) error {
	var buf bytes.Buffer
	err := tmplw.Write(&buf, templateName, parameters)
	if err != nil {
		// unsure if this is reachable. All errors I've been able to produce are found during template parse
		return err
	}

	err = targetDir.WriteFile(ctx, targetPath, buf.Bytes())
	return err
}

func (i *GeneratorImpl) renderString(_ context.Context, parameters map[string]interface{}, templateName string, templateContents string) (string, error) {
	tmpl, err := template.New(templateName).Funcs(sprig.TxtFuncMap()).Parse(templateContents)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, templateName, parameters)
	if err != nil {
		// unsure if this is reachable. All errors I've been able to produce are found during template parse
		return "", err
	}

	return buf.String(), nil
}

// --- response helpers

func (i *GeneratorImpl) errorResponseToplevel(_ context.Context, err error) *api.Response {
	return &api.Response{
		Errors: []error{err},
	}
}

func (i *GeneratorImpl) successResponse(_ context.Context, renderedFiles []api.FileResult) *api.Response {
	return &api.Response{
		Success:       true,
		RenderedFiles: renderedFiles,
	}
}

func (i *GeneratorImpl) errorResponseRender(_ context.Context, renderedFiles []api.FileResult) *api.Response {
	return &api.Response{
		Success:       false,
		RenderedFiles: renderedFiles,
		Errors:        []error{errors.New("an error occurred during rendering, see individual files")},
	}
}

func (i *GeneratorImpl) successFileResult(_ context.Context, relativeFilePath string) api.FileResult {
	return api.FileResult{
		Success:          true,
		RelativeFilePath: relativeFilePath,
	}
}

func (i *GeneratorImpl) errorFileResult(_ context.Context, relativeFilePath string, err error) api.FileResult {
	return api.FileResult{
		Success:          false,
		RelativeFilePath: relativeFilePath,
		Errors:           []error{err},
	}
}
