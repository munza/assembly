package harness

import (
	"fmt"
	"sort"
)

// Harness is one coding-agent harness workers can run under. Kind is the
// herdr agent kind; Args are extra agent-start arguments (none today —
// harnesses run their own defaults).
type Harness struct {
	Kind string
	Args []string
}

var registry = map[string]*Harness{
	"pi": Pi,
}

func For(name string) (*Harness, error) {
	if name == "" {
		name = "pi"
	}
	h, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown harness %q; known: %v", name, Names())
	}
	return h, nil
}

func Names() []string {
	var ns []string
	for n := range registry {
		ns = append(ns, n)
	}
	sort.Strings(ns)
	return ns
}
