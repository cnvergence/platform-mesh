package filter

import "github.com/platform-mesh/qbrtool/internal/models"

// Filter is an interface for filtering project items
type Filter interface {
	// Matches returns true if the item matches the filter
	Matches(item *models.ProjectItem) bool
	// Name returns the name of the filter
	Name() string
}

// ChainMode determines how filters are combined
type ChainMode int

const (
	// ModeAND requires all filters to match
	ModeAND ChainMode = iota
	// ModeOR requires at least one filter to match
	ModeOR
)

// Chain combines multiple filters
type Chain struct {
	filters []Filter
	mode    ChainMode
}

// NewChain creates a new filter chain
func NewChain(filters []Filter, mode ChainMode) *Chain {
	return &Chain{
		filters: filters,
		mode:    mode,
	}
}

// Matches checks if an item matches the filter chain
func (c *Chain) Matches(item *models.ProjectItem) bool {
	if len(c.filters) == 0 {
		return true
	}

	if c.mode == ModeAND {
		for _, f := range c.filters {
			if !f.Matches(item) {
				return false
			}
		}
		return true
	}

	// ModeOR
	for _, f := range c.filters {
		if f.Matches(item) {
			return true
		}
	}
	return false
}

// Name returns the name of the chain
func (c *Chain) Name() string {
	return "chain"
}

// Apply applies the filter chain to a slice of items
func (c *Chain) Apply(items []*models.ProjectItem) []*models.ProjectItem {
	result := make([]*models.ProjectItem, 0, len(items))
	for _, item := range items {
		if c.Matches(item) {
			result = append(result, item)
		}
	}
	return result
}
