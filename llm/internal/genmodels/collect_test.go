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
	"strconv"
	"strings"
	"testing"
)

func TestCollectStrictRejectsSourceFailure(t *testing.T) {
	sources := []catalogSource{
		staticSource("models.dev", []model{validGeneratedModel("anthropic", "claude")}),
		failingSource("additional source", errors.New("service unavailable")),
	}

	_, err := collect(context.Background(), http.DefaultClient, collectOptions{sources: sources})
	if err == nil || !strings.Contains(err.Error(), "additional source: service unavailable") {
		t.Fatalf("collect error = %v, want additional source failure", err)
	}
}

func TestCollectStrictAcceptsCompleteCatalog(t *testing.T) {
	complete := validCompleteCatalog()
	sources := []catalogSource{
		staticSource("models.dev", complete),
	}

	models, err := collect(context.Background(), http.DefaultClient, collectOptions{sources: sources})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(models) != minimumCompleteCatalogModels {
		t.Fatalf("models = %d, want %d", len(models), minimumCompleteCatalogModels)
	}
}

func TestCollectAllowPartialWarnsAndReturnsAvailableModels(t *testing.T) {
	var warnings bytes.Buffer
	sources := []catalogSource{
		staticSource("models.dev", []model{validGeneratedModel("anthropic", "claude")}),
		failingSource("additional source", errors.New("service unavailable")),
	}

	models, err := collect(context.Background(), http.DefaultClient, collectOptions{
		AllowPartial: true,
		Warnings:     &warnings,
		sources:      sources,
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(models) != 1 || models[0].Provider != "anthropic" {
		t.Fatalf("models = %#v, want available source only", models)
	}
	if !strings.Contains(warnings.String(), "additional source: service unavailable") {
		t.Fatalf("warnings = %q, want additional source failure", warnings.String())
	}
}

func TestCollectAllowPartialRejectsWhenAllSourcesFail(t *testing.T) {
	sources := []catalogSource{
		failingSource("models.dev", errors.New("offline")),
	}

	_, err := collect(context.Background(), http.DefaultClient, collectOptions{
		AllowPartial: true,
		Warnings:     io.Discard,
		sources:      sources,
	})
	if err == nil || !strings.Contains(err.Error(), "no catalog source succeeded") {
		t.Fatalf("collect error = %v, want all-sources failure", err)
	}
}

func TestValidateCompleteCatalogInvariants(t *testing.T) {
	complete := validCompleteCatalog()
	if err := validateCatalog(complete, false); err != nil {
		t.Fatalf("validate complete catalog: %v", err)
	}

	t.Run("minimum model count", func(t *testing.T) {
		err := validateCatalog(complete[:minimumCompleteCatalogModels-1], false)
		if err == nil || !strings.Contains(err.Error(), "require at least") {
			t.Fatalf("validation error = %v, want minimum count failure", err)
		}
	})

	t.Run("required provider", func(t *testing.T) {
		missing := append([]model(nil), complete...)
		for index := range missing {
			if missing[index].Provider == "opencode" {
				missing[index].Provider = "anthropic"
			}
		}
		err := validateCatalog(missing, false)
		if err == nil || !strings.Contains(err.Error(), `missing required provider "opencode"`) {
			t.Fatalf("validation error = %v, want missing provider", err)
		}
	})

	t.Run("invalid model limits", func(t *testing.T) {
		invalid := []model{validGeneratedModel("test", "broken")}
		invalid[0].ContextWindow = 0
		err := validateCatalog(invalid, true)
		if err == nil || !strings.Contains(err.Error(), "invalid token limits") {
			t.Fatalf("validation error = %v, want token limit failure", err)
		}
	})

	t.Run("thinking profile requires reasoning", func(t *testing.T) {
		invalid := []model{validGeneratedModel("opencode-go", "mimo-v2.5")}
		err := validateCatalog(invalid, true)
		if err == nil || !strings.Contains(err.Error(), "thinking profile but reasoning is disabled") {
			t.Fatalf("validation error = %v, want reasoning mismatch", err)
		}
	})

	t.Run("thinking profile must be applied", func(t *testing.T) {
		invalid := []model{validGeneratedModel("opencode-go", "mimo-v2.5")}
		invalid[0].Reasoning = true
		invalid[0].Compat.Kind = "openai"
		err := validateCatalog(invalid, true)
		if err == nil || !strings.Contains(err.Error(), "does not match its thinking profile") {
			t.Fatalf("validation error = %v, want unapplied profile mismatch", err)
		}
	})
}

func TestGenerateCatalogFailurePreservesExistingOutput(t *testing.T) {
	output := filepath.Join(t.TempDir(), "catalog.json")
	const existing = "existing catalog\n"
	if err := os.WriteFile(output, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	err := generateCatalog(context.Background(), http.DefaultClient, output, collectOptions{
		sources: []catalogSource{failingSource("models.dev", errors.New("offline"))},
	}, io.Discard)
	if err == nil {
		t.Fatal("generateCatalog succeeded, want failure")
	}
	got, readErr := os.ReadFile(output)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != existing {
		t.Fatalf("output = %q, want preserved %q", got, existing)
	}
}

func TestGenerateCatalogAllowPartialReplacesOutputAtomically(t *testing.T) {
	directory := t.TempDir()
	output := filepath.Join(directory, "catalog.json")
	if err := os.WriteFile(output, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	err := generateCatalog(context.Background(), http.DefaultClient, output, collectOptions{
		AllowPartial: true,
		Warnings:     io.Discard,
		sources: []catalogSource{
			staticSource("models.dev", []model{validGeneratedModel("anthropic", "claude")}),
		},
	}, &stdout)
	if err != nil {
		t.Fatalf("generateCatalog: %v", err)
	}

	generated, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var catalog []catalogModel
	if err := json.Unmarshal(generated, &catalog); err != nil {
		t.Fatalf("decode generated catalog: %v\n%s", err, generated)
	}
	if len(catalog) != 1 || catalog[0].Provider != "anthropic" {
		t.Fatalf("catalog = %#v, want one Anthropic model", catalog)
	}
	if !strings.Contains(stdout.String(), "with 1 models") {
		t.Fatalf("stdout = %q, want model count", stdout.String())
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("output mode = %o, want 644", info.Mode().Perm())
	}
	temporary, err := filepath.Glob(filepath.Join(directory, ".catalog.json.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporary) != 0 {
		t.Fatalf("temporary files remain: %v", temporary)
	}
}

func staticSource(name string, models []model) catalogSource {
	return catalogSource{
		name: name,
		load: func(context.Context, *http.Client) ([]model, error) {
			return append([]model(nil), models...), nil
		},
	}
}

func failingSource(name string, err error) catalogSource {
	return catalogSource{
		name: name,
		load: func(context.Context, *http.Client) ([]model, error) {
			return nil, err
		},
	}
}

func validGeneratedModel(provider, id string) model {
	return model{
		ID:            id,
		Name:          id,
		Provider:      provider,
		Protocol:      "openai-completions",
		BaseURL:       "https://example.com/v1",
		Input:         []string{"text"},
		ContextWindow: 4096,
		MaxTokens:     1024,
	}
}

func validCompleteCatalog() []model {
	models := make([]model, minimumCompleteCatalogModels)
	for index := range models {
		provider := requiredCompleteCatalogProviders[index%len(requiredCompleteCatalogProviders)]
		models[index] = validGeneratedModel(provider, provider+"-model-"+strconv.Itoa(index+1))
	}
	return models
}
