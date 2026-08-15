package tools

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

func DefaultDefinitions() []Definition {
	return []Definition{
		ReadDefinition(),
		WriteDefinition(),
		CreateDefinition(),
		DeleteDefinition(),
		SaveDefinition(),
	}
}
