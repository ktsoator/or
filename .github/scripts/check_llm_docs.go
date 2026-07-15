package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type catalogModel struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Protocol string `json:"protocol"`
}

var (
	markdownLink = regexp.MustCompile(`\[[^]]*\]\(([^)]+)\)`)
	modelLookup  = regexp.MustCompile(`(?s)llm\.(?:GetModel|LookupModel)\(\s*"([^"]+)"\s*,\s*"([^"]+)"\s*\)`)
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "LLM documentation check failed:", err)
		os.Exit(1)
	}
	fmt.Println("LLM documentation checks passed")
}

func run() error {
	models, err := readCatalog("llm/catalog.generated.json")
	if err != nil {
		return err
	}

	var markdownFiles []string
	err = filepath.WalkDir("docs/llm", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".md") {
			markdownFiles = append(markdownFiles, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(markdownFiles)

	for _, path := range markdownFiles {
		if err := checkBilingualPair(path); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := checkLocalLinks(path, string(data)); err != nil {
			return err
		}
		if err := checkModelLookups(path, string(data), models); err != nil {
			return err
		}
	}

	if err := checkCatalogMarker("docs/llm/support-matrix.md", models); err != nil {
		return err
	}
	if err := checkCatalogMarker("docs/llm/support-matrix.zh.md", models); err != nil {
		return err
	}
	return checkAPIReference()
}

func readCatalog(path string) (map[string]catalogModel, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []catalogModel
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	models := make(map[string]catalogModel, len(entries))
	for _, model := range entries {
		models[model.Provider+"\x00"+model.ID] = model
	}
	return models, nil
}

func checkBilingualPair(path string) error {
	var pair string
	if strings.HasSuffix(path, ".zh.md") {
		pair = strings.TrimSuffix(path, ".zh.md") + ".md"
	} else {
		pair = strings.TrimSuffix(path, ".md") + ".zh.md"
	}
	if _, err := os.Stat(pair); err != nil {
		return fmt.Errorf("%s has no bilingual pair %s", path, pair)
	}
	return nil
}

func checkLocalLinks(source, body string) error {
	for _, match := range markdownLink.FindAllStringSubmatch(body, -1) {
		target := strings.TrimSpace(match[1])
		target = strings.Trim(target, "<>")
		if target == "" || strings.HasPrefix(target, "#") || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") || strings.ContainsAny(target, " ,") {
			continue
		}
		if index := strings.IndexByte(target, '#'); index >= 0 {
			target = target[:index]
		}
		if target == "" {
			continue
		}
		resolved := filepath.Clean(filepath.Join(filepath.Dir(source), filepath.FromSlash(target)))
		if _, err := os.Stat(resolved); err != nil {
			return fmt.Errorf("%s links to missing path %s", source, target)
		}
	}
	return nil
}

func checkModelLookups(path, body string, models map[string]catalogModel) error {
	for _, match := range modelLookup.FindAllStringSubmatch(body, -1) {
		key := match[1] + "\x00" + match[2]
		if _, ok := models[key]; !ok {
			return fmt.Errorf("%s references unknown catalog model %s/%s", path, match[1], match[2])
		}
	}
	return nil
}

func checkCatalogMarker(path string, models map[string]catalogModel) error {
	counts := map[string]int{}
	for _, model := range models {
		counts[model.Protocol]++
	}
	runnable := counts["openai-completions"] + counts["anthropic-messages"]
	expected := fmt.Sprintf(
		"<!-- catalog-stats: total=%d runnable=%d openai-completions=%d anthropic-messages=%d openai-responses=%d google-generative-ai=%d mistral-conversations=%d -->",
		len(models), runnable, counts["openai-completions"], counts["anthropic-messages"], counts["openai-responses"], counts["google-generative-ai"], counts["mistral-conversations"],
	)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), expected) {
		return fmt.Errorf("%s catalog marker is stale; expected %s", path, expected)
	}
	return nil
}

func checkAPIReference() error {
	required := []string{
		"Complete", "Stream", "NewClient", "Prompt", "PromptWithSystem",
		"MarshalMessage", "UnmarshalMessage", "TransformMessages",
		"NewTool", "MustTool", "DecodeToolCall", "ValidateToolCall",
		"LookupModel", "GetModel", "GetProviders", "GetModels",
		"GetRunnableModels", "SupportsProtocol", "CalculateCost",
		"NewAdapterRegistry", "NewProviderRegistry", "NewModelRegistry",
		"DefaultProviderRegistry", "NewStreamWriter", "IsContextOverflow",
	}
	for _, path := range []string{"docs/llm/api-reference.md", "docs/llm/api-reference.zh.md"} {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, name := range required {
			if !strings.Contains(string(data), "`"+name) {
				return errors.New(path + " does not mention public API " + name)
			}
		}
	}
	return nil
}
