package compounds

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

type Registry struct {
	mu        sync.RWMutex
	compounds []Compound
	byName    map[string]*Compound
	aliasMap  map[string]*Compound
}

func Default() *Registry {
	r := &Registry{compounds: make([]Compound, len(seedCompounds))}
	copy(r.compounds, seedCompounds)
	r.buildIndex()
	return r
}

func (r *Registry) buildIndex() {
	r.byName = make(map[string]*Compound, len(r.compounds))
	r.aliasMap = make(map[string]*Compound, len(r.compounds)*5)
	for i := range r.compounds {
		c := &r.compounds[i]
		r.byName[norm(c.Name)] = c
		r.aliasMap[norm(c.DisplayName)] = c
		for _, alias := range c.Aliases {
			r.aliasMap[norm(alias)] = c
		}
	}
}

func (r *Registry) FindByName(name string) (*Compound, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.byName[norm(name)]
	return c, ok
}

func (r *Registry) FindByAlias(raw string) (*Compound, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := norm(raw)
	if c, ok := r.byName[key]; ok {
		return c, true
	}
	if c, ok := r.aliasMap[key]; ok {
		return c, true
	}
	return nil, false
}

func (r *Registry) Search(query string) []Compound {
	r.mu.RLock()
	defer r.mu.RUnlock()
	q := norm(query)
	var results []Compound
	for _, c := range r.compounds {
		if strings.Contains(norm(c.Name), q) ||
			strings.Contains(norm(c.DisplayName), q) ||
			strings.Contains(norm(c.Description), q) ||
			anyContains(c.Aliases, q) {
			results = append(results, c)
		}
	}
	return results
}

func (r *Registry) All() []Compound {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Compound, len(r.compounds))
	copy(out, r.compounds)
	return out
}

func (r *Registry) ByCategory(cat Category) []Compound {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var results []Compound
	for _, c := range r.compounds {
		if c.Category == cat {
			results = append(results, c)
		}
	}
	return results
}

func (r *Registry) Categories() []Category {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := make(map[Category]bool)
	var cats []Category
	for _, c := range r.compounds {
		if !seen[c.Category] {
			seen[c.Category] = true
			cats = append(cats, c.Category)
		}
	}
	return cats
}

func (r *Registry) Add(c Compound) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byName[norm(c.Name)]; ok {
		return
	}
	r.compounds = append(r.compounds, c)
	ptr := &r.compounds[len(r.compounds)-1]
	r.byName[norm(ptr.Name)] = ptr
	r.aliasMap[norm(ptr.DisplayName)] = ptr
	for _, alias := range ptr.Aliases {
		r.aliasMap[norm(alias)] = ptr
	}
}

func (r *Registry) AddAlias(canonicalName, alias string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byName[norm(canonicalName)]
	if !ok {
		return false
	}
	c.Aliases = append(c.Aliases, alias)
	r.aliasMap[norm(alias)] = c
	return true
}

func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.compounds)
}

func (r *Registry) LoadFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read compounds file: %w", err)
	}
	var comps []Compound
	if err := json.Unmarshal(data, &comps); err != nil {
		return 0, fmt.Errorf("parse compounds file: %w", err)
	}
	added := 0
	for _, c := range comps {
		if c.Name == "" {
			continue
		}
		if _, exists := r.FindByName(c.Name); !exists {
			r.Add(c)
			added++
		}
	}
	return added, nil
}

func (r *Registry) SaveFile(path string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	data, err := json.MarshalIndent(r.compounds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func NewEmpty() *Registry {
	r := &Registry{}
	r.buildIndex()
	return r
}

func norm(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func anyContains(strs []string, needle string) bool {
	for _, s := range strs {
		if strings.Contains(norm(s), needle) {
			return true
		}
	}
	return false
}
