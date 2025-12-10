package analyzer

import "github.com/platform-mesh/qbrtool/internal/models"

// Analyzer is an interface for analyzing project items
type Analyzer interface {
	// Name returns the name of the analyzer
	Name() string
	// Analyze runs the analysis on the given items
	Analyze(items []*models.ProjectItem) *models.AnalysisResult
}

// Registry holds registered analyzers
type Registry struct {
	analyzers map[string]Analyzer
}

// NewRegistry creates a new analyzer registry
func NewRegistry() *Registry {
	return &Registry{
		analyzers: make(map[string]Analyzer),
	}
}

// Register adds an analyzer to the registry
func (r *Registry) Register(a Analyzer) {
	r.analyzers[a.Name()] = a
}

// Get returns an analyzer by name
func (r *Registry) Get(name string) (Analyzer, bool) {
	a, ok := r.analyzers[name]
	return a, ok
}

// All returns all registered analyzers
func (r *Registry) All() []Analyzer {
	result := make([]Analyzer, 0, len(r.analyzers))
	for _, a := range r.analyzers {
		result = append(result, a)
	}
	return result
}

// NewDefaultRegistry creates a registry with all default analyzers
func NewDefaultRegistry(ossOrgs []string) *Registry {
	r := NewRegistry()
	r.Register(NewCVEAnalyzer())
	r.Register(NewOSSAnalyzer(ossOrgs))
	r.Register(NewMonitoringAnalyzer())
	r.Register(NewLifecycleAnalyzer())
	r.Register(NewSecurityAnalyzer())
	return r
}
