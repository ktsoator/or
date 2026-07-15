package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
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
	if err := checkAPIReference(); err != nil {
		return err
	}
	return checkRecipes()
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

func checkRecipes() error {
	recipes := []string{
		"basic-completion", "streaming-chat", "conversation-persistence",
		"vision", "reasoning", "tool-loop", "model-switching",
		"provider-discovery", "custom-gateway", "custom-client",
		"observability", "error-handling", "mock-testing",
	}
	index, err := os.ReadFile("docs/llm/recipes/README.md")
	if err != nil {
		return err
	}
	localizedIndex, err := os.ReadFile("docs/llm/recipes/README.zh.md")
	if err != nil {
		return err
	}

	for _, name := range recipes {
		link := name + ".md"
		if !strings.Contains(string(index), "("+link+")") ||
			!strings.Contains(string(localizedIndex), "("+link+")") {
			return fmt.Errorf("recipe index does not link both languages to %s", link)
		}
		for _, suffix := range []string{".md", ".zh.md"} {
			path := filepath.Join("docs/llm/recipes", name+suffix)
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			body := string(data)
			if len(data) < 1500 {
				return fmt.Errorf("%s is too short for a task guide", path)
			}
			if !strings.Contains(body, "```go") {
				return fmt.Errorf("%s has no Go example", path)
			}
			code, err := firstGoBlock(body)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			if _, err := parser.ParseFile(token.NewFileSet(), path, code, parser.AllErrors); err != nil {
				return fmt.Errorf("%s first Go program has invalid syntax: %w", path, err)
			}
			if strings.Count(body, "\n## ") < 3 {
				return fmt.Errorf("%s needs purpose, implementation, and operational guidance", path)
			}
			for _, fragment := range []string{"## Core code", "## Program skeleton", "## 核心代码", "## 程序骨架"} {
				if strings.Contains(body, fragment) {
					return fmt.Errorf("%s still contains snippet-only section %q", path, fragment)
				}
			}
		}
	}
	return compileRecipeExamples(recipes)
}

func firstGoBlock(body string) (string, error) {
	const opening = "```go\n"
	start := strings.Index(body, opening)
	if start < 0 {
		return "", errors.New("Go code fence is missing")
	}
	rest := body[start+len(opening):]
	end := strings.Index(rest, "\n```")
	if end < 0 {
		return "", errors.New("Go code fence is not closed")
	}
	return rest[:end] + "\n", nil
}

func compileRecipeExamples(recipes []string) error {
	root, err := filepath.Abs(".")
	if err != nil {
		return err
	}
	temp, err := os.MkdirTemp("", "llm-recipe-examples-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)

	goMod := fmt.Sprintf(`module llmrecipedocs

go 1.24

require github.com/ktsoator/or v0.0.0

replace github.com/ktsoator/or => %s
`, filepath.ToSlash(root))
	if err := os.WriteFile(filepath.Join(temp, "go.mod"), []byte(goMod), 0o600); err != nil {
		return err
	}
	goSum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(temp, "go.sum"), goSum, 0o600); err != nil {
		return err
	}

	for _, name := range recipes {
		data, err := os.ReadFile(filepath.Join("docs/llm/recipes", name+".md"))
		if err != nil {
			return err
		}
		code, err := firstGoBlock(string(data))
		if err != nil {
			return err
		}
		dir := filepath.Join(temp, name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		filename := "main.go"
		if strings.HasPrefix(strings.TrimSpace(code), "package myapp_test") {
			filename = "example_test.go"
			if err := os.WriteFile(filepath.Join(dir, "doc.go"), []byte("package myapp\n"), 0o600); err != nil {
				return err
			}
		}
		if err := os.WriteFile(filepath.Join(dir, filename), []byte(code), 0o600); err != nil {
			return err
		}
	}

	command := exec.Command("go", "test", "-mod=mod", "-run=^$", "./...")
	command.Dir = temp
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("recipe Go programs do not compile: %w\n%s", err, output)
	}
	return nil
}
