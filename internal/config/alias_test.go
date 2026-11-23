package config

import "testing"

func TestResolveAlias(t *testing.T) {
	c := &Config{
		Aliases: map[string]Alias{
			"code": {Model: "qwen:14b", Machine: "firmino"},
		},
	}
	model, machine, alias := c.ResolveAlias("code", "")
	if alias != "code" || model != "qwen:14b" || machine != "firmino" {
		t.Fatalf("got %q %q %q", model, machine, alias)
	}
	model, machine, alias = c.ResolveAlias("code", "mane")
	if alias != "code" || model != "qwen:14b" || machine != "mane" {
		t.Fatalf("explicit machine wins: %q %q", machine, model)
	}
}

func TestAliasValidation(t *testing.T) {
	c := &Config{
		Router:   RouterConfig{Listen: "127.0.0.1:8787"},
		Machines: []Machine{{Name: "a", Address: "http://127.0.0.1:1", Roles: []string{"fast"}, DefaultModel: "m:8b", MaxConcurrent: 1, NumCtx: 8192}},
		Routing:  RoutingConfig{AutoRole: "fast", FastRole: "fast"},
		Aliases:  map[string]Alias{"bad:name": {Model: "m:8b"}},
	}
	if _, err := c.validate(); err == nil {
		t.Fatal("invalid alias name")
	}
}
