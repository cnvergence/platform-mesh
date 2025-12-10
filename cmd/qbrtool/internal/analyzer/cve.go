package analyzer

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/platform-mesh/qbrtool/internal/models"
)

var cvePattern = regexp.MustCompile(`CVE-\d{4}-\d{4,}`)

// CVEAnalyzer detects CVE references in project items
type CVEAnalyzer struct{}

// NewCVEAnalyzer creates a new CVE analyzer
func NewCVEAnalyzer() *CVEAnalyzer {
	return &CVEAnalyzer{}
}

// Name returns the analyzer name
func (a *CVEAnalyzer) Name() string {
	return "cve"
}

// Analyze searches for CVE patterns in items
func (a *CVEAnalyzer) Analyze(items []*models.ProjectItem) *models.AnalysisResult {
	var matches []models.MatchedItem
	cveSet := make(map[string]bool)

	for _, item := range items {
		matchInfo := a.findCVEs(item)
		if matchInfo != nil {
			// Extract CVE IDs
			cves := cvePattern.FindAllString(matchInfo.MatchedText, -1)
			for _, cve := range cves {
				cveSet[strings.ToUpper(cve)] = true
			}

			matches = append(matches, models.MatchedItem{
				Item:      *item,
				MatchInfo: *matchInfo,
			})
		}
	}

	// Build unique sorted CVE list
	var cveList []string
	for cve := range cveSet {
		cveList = append(cveList, cve)
	}
	sort.Strings(cveList)

	return &models.AnalysisResult{
		Type:  "cve",
		Items: matches,
		Summary: models.CVEAnalysis{
			CVEIDs: cveList,
			Count:  len(cveList),
		},
		Timestamp: time.Now(),
	}
}

// findCVEs searches for CVE patterns in an item
func (a *CVEAnalyzer) findCVEs(item *models.ProjectItem) *models.MatchInfo {
	// Check title first (highest priority)
	if cves := cvePattern.FindAllString(item.Title, -1); len(cves) > 0 {
		return &models.MatchInfo{
			MatchedIn:   "title",
			MatchedText: strings.Join(cves, ", "),
			Confidence:  1.0,
		}
	}

	// Check body
	if cves := cvePattern.FindAllString(item.Body, -1); len(cves) > 0 {
		return &models.MatchInfo{
			MatchedIn:   "body",
			MatchedText: strings.Join(cves, ", "),
			Confidence:  0.9,
		}
	}

	// Check labels
	for _, label := range item.Labels {
		if cves := cvePattern.FindAllString(label, -1); len(cves) > 0 {
			return &models.MatchInfo{
				MatchedIn:   "labels",
				MatchedText: strings.Join(cves, ", "),
				Confidence:  1.0,
			}
		}
	}

	// Check URL (sometimes CVE IDs appear in linked URLs)
	if cves := cvePattern.FindAllString(item.URL, -1); len(cves) > 0 {
		return &models.MatchInfo{
			MatchedIn:   "url",
			MatchedText: strings.Join(cves, ", "),
			Confidence:  0.8,
		}
	}

	return nil
}
