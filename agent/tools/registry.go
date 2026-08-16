package tools

import (
	"sort"
	"strings"
)

// Registry keeps the provider-facing tool definitions in one place.
type Registry struct {
	definitions []Definition
	byName      map[string]Definition
}

func NewRegistry() *Registry {
	definitions := DefaultDefinitions()
	byName := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		byName[definition.Name] = definition
	}
	return &Registry{definitions: definitions, byName: byName}
}

func (r *Registry) Definitions() []Definition {
	return append([]Definition(nil), r.definitions...)
}

func (r *Registry) Find(name string) (Definition, bool) {
	definition, ok := r.byName[name]
	return definition, ok
}

// ToolSummary is the small, searchable representation exposed by the
// progressive tool discovery flow. The full schema is returned only after a
// caller loads the tool by key.
type ToolSummary struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// Search returns summaries whose name, label, or description contains every
// non-empty query term. Results retain a deterministic registry order.
func (r *Registry) Search(query string) []ToolSummary {
	terms := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	results := make([]ToolSummary, 0, len(r.definitions))
	for _, definition := range r.definitions {
		searchable := strings.ToLower(strings.Join([]string{definition.Name, definition.Label, definition.Description}, " "))
		matched := true
		for _, term := range terms {
			if !strings.Contains(searchable, term) {
				matched = false
				break
			}
		}
		if matched {
			results = append(results, ToolSummary{Key: definition.Name, Label: definition.Label, Description: definition.Description})
		}
	}
	return results
}

// Loaded returns a copy of the definition identified by key. Keys are stable
// tool names, which are also the names sent to the provider.
func (r *Registry) Loaded(key string) (Definition, bool) {
	definition, ok := r.byName[strings.TrimSpace(key)]
	return definition, ok
}

// LoadedDefinitions returns definitions in key order, making provider
// requests deterministic even when the caller stores them in a map.
func LoadedDefinitions(loaded map[string]Definition) []Definition {
	keys := make([]string, 0, len(loaded))
	for key := range loaded {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	definitions := make([]Definition, 0, len(keys))
	for _, key := range keys {
		definition := loaded[key]
		if definition.Name == "" {
			definition.Name = key
		}
		definitions = append(definitions, definition)
	}
	return definitions
}

func DefaultDefinitions() []Definition {
	return []Definition{
		ReadDefinition(),
		WriteDefinition(),
		CreateDefinition(),
		DeleteDefinition(),
		SaveDefinition(),
	}
}
