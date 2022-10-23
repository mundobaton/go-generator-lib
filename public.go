package generatorlib

import (
	"context"
	"github.com/mundobaton/go-generator-lib/api"
	"github.com/mundobaton/go-generator-lib/internal/implementation"
	"github.com/mundobaton/go-generator-lib/internal/logfacade"
)

var Instance api.Api

func init() {
	Instance = &logfacade.GeneratorLogfacade{Wrapped: &implementation.GeneratorImpl{}}
}

func FindGeneratorNames(ctx context.Context, sourceBaseDir string) ([]string, error) {
	return Instance.FindGeneratorNames(ctx, sourceBaseDir)
}

func ObtainGeneratorSpec(ctx context.Context, sourceBaseDir string, generatorName string) (*api.GeneratorSpec, error) {
	return Instance.ObtainGeneratorSpec(ctx, sourceBaseDir, generatorName)
}

func WriteRenderSpecWithDefaults(ctx context.Context, request *api.Request, generatorName string) *api.Response {
	return Instance.WriteRenderSpecWithDefaults(ctx, request, generatorName)
}

func WriteRenderSpecWithValues(ctx context.Context, request *api.Request, generatorName string, parameters map[string]interface{}) *api.Response {
	return Instance.WriteRenderSpecWithValues(ctx, request, generatorName, parameters)
}

func Render(ctx context.Context, request *api.Request) *api.Response {
	return Instance.Render(ctx, request)
}
