package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderInspectionRouteSpecsDescribeFixedSharedAndSpecialRoutes(t *testing.T) {
	routes := providerInspectionRouteSpecs()
	catalog := make(map[string]sourceProvider)
	for _, route := range routes {
		if _, exists := catalog[route.Source]; !exists {
			catalog[route.Source] = sourceProvider{Models: map[string]sourceModel{}}
		}
	}

	index, providers, err := buildProviderInspections(catalog, nil, routes)
	if err != nil {
		t.Fatalf("buildProviderInspections: %v", err)
	}
	if index.ProviderCount != len(routes) {
		t.Fatalf("provider count = %d, want %d", index.ProviderCount, len(routes))
	}

	xiaomi := requireProviderInspection(t, providers, "xiaomi-token-plan-cn")
	if xiaomi.Route.Kind != "fixed" || xiaomi.Route.Protocol != "openai-completions" || xiaomi.Route.BaseURL != "https://token-plan-cn.xiaomimimo.com/v1" {
		t.Fatalf("Xiaomi route = %#v, want fixed OpenAI route", xiaomi.Route)
	}

	zai := requireProviderInspection(t, providers, "zai")
	if !equalStrings(zai.Route.SharedWith, []string{"zai-coding-cn"}) {
		t.Fatalf("ZAI shared routes = %v, want zai-coding-cn", zai.Route.SharedWith)
	}
	zaiCN := requireProviderInspection(t, providers, "zai-coding-cn")
	if !equalStrings(zaiCN.Route.SharedWith, []string{"zai"}) {
		t.Fatalf("ZAI CN shared routes = %v, want zai", zaiCN.Route.SharedWith)
	}

	openCode := requireProviderInspection(t, providers, "opencode-go")
	if openCode.Route.Kind != "opencode" || openCode.Route.Protocol != "" || openCode.Route.BaseURL != "" {
		t.Fatalf("OpenCode route = %#v, want dynamic protocol route", openCode.Route)
	}
	copilot := requireProviderInspection(t, providers, "github-copilot")
	if copilot.Route.Kind != "copilot" || copilot.Route.Protocol != "" || copilot.Route.BaseURL != "" {
		t.Fatalf("Copilot route = %#v, want dynamic protocol route", copilot.Route)
	}
}

func TestWriteProviderInspectionCreatesAndReplacesOwnedProviderDirectory(t *testing.T) {
	target := filepath.Join(t.TempDir(), "inspection")
	catalog := map[string]sourceProvider{
		"shared": {Models: map[string]sourceModel{
			"filtered": {},
			"kept":     inspectionTestSourceModel(),
		}},
	}
	routes := []inspectionRouteSpec{
		{Source: "shared", Provider: "first", Kind: "fixed", Protocol: "openai-completions", BaseURL: "https://example.com/v1"},
		{Source: "shared", Provider: "second", Kind: "fixed", Protocol: "openai-completions", BaseURL: "https://example.net/v1"},
	}
	models := []model{validGeneratedModel("first", "kept")}

	if err := writeProviderInspectionWithRoutes(target, catalog, models, routes); err != nil {
		t.Fatalf("writeProviderInspectionWithRoutes: %v", err)
	}
	firstAfterPath := filepath.Join(target, "first", "after.json")
	firstAfter := mustReadFile(t, firstAfterPath)

	var index inspectionIndex
	mustDecodeJSONFile(t, filepath.Join(target, "index.json"), &index)
	if index.ProviderCount != 2 || index.OutputModelCount != 1 {
		t.Fatalf("index = %#v, want two providers and one output model", index)
	}
	if index.BeforeSemantics == "" {
		t.Fatal("index does not describe before.json semantics")
	}

	var route inspectionRoute
	mustDecodeJSONFile(t, filepath.Join(target, "first", "route.json"), &route)
	if !equalStrings(route.SharedWith, []string{"second"}) || !equalStrings(route.OutputProtocols, []string{"openai-completions"}) {
		t.Fatalf("first route = %#v, want shared OpenAI output", route)
	}

	var before inspectionInput
	mustDecodeJSONFile(t, filepath.Join(target, "first", "before.json"), &before)
	if len(before.Models) != 2 || before.Models["filtered"].ToolCall {
		t.Fatalf("before models = %#v, want kept and pre-filter input", before.Models)
	}
	var after []catalogModel
	mustDecodeJSONFile(t, firstAfterPath, &after)
	if len(after) != 1 || after[0].Provider != "first" || after[0].ID != "kept" {
		t.Fatalf("after models = %#v, want final catalog model", after)
	}
	var emptyAfter []catalogModel
	mustDecodeJSONFile(t, filepath.Join(target, "second", "after.json"), &emptyAfter)
	if len(emptyAfter) != 0 {
		t.Fatalf("second after models = %#v, want empty array", emptyAfter)
	}

	stale := filepath.Join(target, "stale.txt")
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeProviderInspectionWithRoutes(target, catalog, models, routes); err != nil {
		t.Fatalf("replace provider inspection: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale file still exists: %v", err)
	}
	if got := mustReadFile(t, firstAfterPath); !bytes.Equal(got, firstAfter) {
		t.Fatal("identical inspection input produced different after.json")
	}
}

func TestWriteInspectionDirectoryRefusesUnownedTargetsAndPreservesPreviousSnapshot(t *testing.T) {
	parent := t.TempDir()
	unowned := filepath.Join(parent, "unowned")
	if err := os.Mkdir(unowned, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(unowned, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := writeInspectionDirectory(unowned, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "unowned inspection directory") {
		t.Fatalf("unowned directory error = %v", err)
	}
	if got := string(mustReadFile(t, sentinel)); got != "keep" {
		t.Fatalf("unowned sentinel = %q, want keep", got)
	}

	owned := filepath.Join(parent, "owned")
	if err := writeInspectionDirectory(owned, func(root string) error {
		return os.WriteFile(filepath.Join(root, "previous.txt"), []byte("previous"), 0o644)
	}); err != nil {
		t.Fatal(err)
	}
	err = writeInspectionDirectory(owned, func(string) error { return errors.New("build failed") })
	if err == nil || !strings.Contains(err.Error(), "build failed") {
		t.Fatalf("failed replacement error = %v", err)
	}
	if got := string(mustReadFile(t, filepath.Join(owned, "previous.txt"))); got != "previous" {
		t.Fatalf("previous snapshot = %q, want previous", got)
	}

	volumeRoot := filepath.VolumeName(parent) + string(filepath.Separator)
	for _, unsafe := range []string{"", ".", volumeRoot} {
		t.Run("unsafe_"+strings.ReplaceAll(unsafe, string(filepath.Separator), "root"), func(t *testing.T) {
			err := writeInspectionDirectory(unsafe, func(string) error { return nil })
			if err == nil || !strings.Contains(err.Error(), "unsafe inspection directory") {
				t.Fatalf("unsafe directory %q error = %v", unsafe, err)
			}
		})
	}
}

func TestWriteInspectionDirectoryRejectsFilesAndSymlinks(t *testing.T) {
	parent := t.TempDir()
	fileTarget := filepath.Join(parent, "file")
	if err := os.WriteFile(fileTarget, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeInspectionDirectory(fileTarget, func(string) error { return nil }); err == nil || !strings.Contains(err.Error(), "non-directory") {
		t.Fatalf("file target error = %v", err)
	}
	if got := string(mustReadFile(t, fileTarget)); got != "keep" {
		t.Fatalf("file target = %q, want keep", got)
	}

	realDirectory := filepath.Join(parent, "real")
	if err := os.Mkdir(realDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	targetSymlink := filepath.Join(parent, "target-link")
	if err := os.Symlink(realDirectory, targetSymlink); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := writeInspectionDirectory(targetSymlink, func(string) error { return nil }); err == nil || !strings.Contains(err.Error(), "non-directory") {
		t.Fatalf("target symlink error = %v", err)
	}

	markerDirectory := filepath.Join(parent, "marker-link")
	if err := os.Mkdir(markerDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	markerSource := filepath.Join(parent, "marker-source")
	if err := os.WriteFile(markerSource, []byte(inspectionMarkerContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(markerSource, filepath.Join(markerDirectory, inspectionMarkerName)); err != nil {
		t.Skipf("marker symlink unavailable: %v", err)
	}
	if err := writeInspectionDirectory(markerDirectory, func(string) error { return nil }); err == nil || !strings.Contains(err.Error(), "unowned inspection directory") {
		t.Fatalf("marker symlink error = %v", err)
	}
}

func TestBuildProviderInspectionsRejectsMissingSourceAndUnknownOutputProvider(t *testing.T) {
	routes := []inspectionRouteSpec{{Source: "missing", Provider: "known", Kind: "fixed"}}
	_, _, err := buildProviderInspections(nil, nil, routes)
	if err == nil || !strings.Contains(err.Error(), `references missing source "missing"`) {
		t.Fatalf("missing source error = %v", err)
	}

	catalog := map[string]sourceProvider{"source": {Models: map[string]sourceModel{}}}
	routes = []inspectionRouteSpec{{Source: "source", Provider: "known", Kind: "fixed"}}
	_, _, err = buildProviderInspections(catalog, []model{validGeneratedModel("unknown", "model")}, routes)
	if err == nil || !strings.Contains(err.Error(), "has no inspection route") {
		t.Fatalf("unknown provider error = %v", err)
	}

	specialRoutes := []inspectionRouteSpec{{Source: "optional", Provider: "special", Kind: "opencode"}}
	_, providers, err := buildProviderInspections(nil, nil, specialRoutes)
	if err != nil {
		t.Fatalf("missing optional special source: %v", err)
	}
	if len(providers) != 1 || providers[0].Route.SourcePresent || len(providers[0].Before.Models) != 0 {
		t.Fatalf("missing special source inspection = %#v", providers)
	}
}

func TestGenerateInspectionFetchesModelsDevOnceAndDoesNotWriteCatalog(t *testing.T) {
	catalog := completeInspectionTestCatalog()
	encoded, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(encoded)),
		}, nil
	})}
	directory := t.TempDir()
	output := filepath.Join(directory, "catalog.generated.json")
	if err := os.WriteFile(output, []byte("unchanged"), 0o644); err != nil {
		t.Fatal(err)
	}
	inspectionDirectory := filepath.Join(directory, "inspection")
	if err := generateInspection(context.Background(), client, inspectionDirectory, io.Discard); err != nil {
		t.Fatalf("generateInspection: %v", err)
	}
	if requests != 1 {
		t.Fatalf("models.dev requests = %d, want 1", requests)
	}
	if got := string(mustReadFile(t, output)); got != "unchanged" {
		t.Fatalf("catalog output = %q, want unchanged", got)
	}
	var index inspectionIndex
	mustDecodeJSONFile(t, filepath.Join(inspectionDirectory, "index.json"), &index)
	if index.ProviderCount != len(providerInspectionRouteSpecs()) || index.OutputModelCount < minimumCompleteCatalogModels {
		t.Fatalf("inspection index = %#v, want complete provider snapshot", index)
	}
	var openCodeRoute inspectionRoute
	mustDecodeJSONFile(t, filepath.Join(inspectionDirectory, "opencode-go", "route.json"), &openCodeRoute)
	if !equalStrings(openCodeRoute.OutputProtocols, []string{"anthropic-messages", "openai-completions"}) {
		t.Fatalf("OpenCode output protocols = %v", openCodeRoute.OutputProtocols)
	}
	var copilotRoute inspectionRoute
	mustDecodeJSONFile(t, filepath.Join(inspectionDirectory, githubCopilotProvider, "route.json"), &copilotRoute)
	if !equalStrings(copilotRoute.OutputProtocols, []string{"anthropic-messages", "openai-completions"}) {
		t.Fatalf("Copilot output protocols = %v", copilotRoute.OutputProtocols)
	}
}

func completeInspectionTestCatalog() map[string]sourceProvider {
	catalog := make(map[string]sourceProvider)
	for _, rule := range providerRules {
		source := catalog[rule.Source]
		if source.Models == nil {
			source.Models = make(map[string]sourceModel)
		}
		source.Models["route-model"] = inspectionTestSourceModel()
		catalog[rule.Source] = source
	}

	openCodeModels := make(map[string]sourceModel, minimumCompleteCatalogModels)
	for index := 0; index < minimumCompleteCatalogModels; index++ {
		openCodeModels["model-"+formatInspectionTestIndex(index)] = inspectionTestSourceModel()
	}
	catalog["opencode"] = sourceProvider{Models: openCodeModels}
	catalog["opencode-go"] = sourceProvider{Models: map[string]sourceModel{
		"anthropic-route":   inspectionTestSourceModelWithNPM("@ai-sdk/anthropic"),
		"openai-route":      inspectionTestSourceModel(),
		"unsupported-route": inspectionTestSourceModelWithNPM("@ai-sdk/openai"),
	}}
	catalog[githubCopilotSource] = sourceProvider{Models: map[string]sourceModel{
		"claude-sonnet-4-6": inspectionTestSourceModel(),
		"gpt-4o":            inspectionTestSourceModel(),
		"gpt-5-excluded":    inspectionTestSourceModel(),
		"oswe-excluded":     inspectionTestSourceModel(),
	}}
	return catalog
}

func inspectionTestSourceModel() sourceModel {
	candidate := sourceModel{ToolCall: true}
	candidate.Limit.Context = 4096
	candidate.Limit.Output = 1024
	return candidate
}

func inspectionTestSourceModelWithNPM(npm string) sourceModel {
	candidate := inspectionTestSourceModel()
	candidate.Provider.NPM = npm
	return candidate
}

func formatInspectionTestIndex(index int) string {
	const digits = "0123456789"
	return string([]byte{
		digits[(index/100)%10],
		digits[(index/10)%10],
		digits[index%10],
	})
}

func requireProviderInspection(t *testing.T, providers []providerInspection, provider string) providerInspection {
	t.Helper()
	for _, candidate := range providers {
		if candidate.Route.Provider == provider {
			return candidate
		}
	}
	t.Fatalf("missing provider inspection %q", provider)
	return providerInspection{}
}

func mustDecodeJSONFile(t *testing.T, path string, target any) {
	t.Helper()
	if err := json.Unmarshal(mustReadFile(t, path), target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
