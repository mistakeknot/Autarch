// Package selector provides intelligent hunter selection based on domain detection.
package selector

// Domain represents a research domain with associated keywords and hunters.
type Domain struct {
	Name        string
	Keywords    []string
	Hunters     []string // Preferred API hunters for this domain
	Description string
}

// Domains is the taxonomy of research domains.
var Domains = []Domain{
	{
		Name: "medical",
		Keywords: []string{
			"health", "medical", "clinical", "patient", "disease", "diagnosis",
			"treatment", "therapy", "hospital", "doctor", "medicine", "pharmaceutical",
			"drug", "symptom", "condition", "healthcare", "fda", "clinical trial",
			"biomedical", "pathology", "oncology", "cardiology", "neurology",
		},
		Hunters:     []string{"pubmed", "openalex"},
		Description: "Medical research, clinical studies, healthcare",
	},
	{
		Name: "nutrition",
		Keywords: []string{
			"nutrition", "diet", "food", "allergy", "allergen", "ingredient",
			"recipe", "meal", "calorie", "nutrient", "vitamin", "protein",
			"carbohydrate", "fat", "fiber", "dietary", "eating", "cuisine",
			"cooking", "restaurant", "menu", "usda", "fda",
		},
		Hunters:     []string{"usda-nutrition", "pubmed"},
		Description: "Food, nutrition, allergies, recipes",
	},
	{
		Name: "legal",
		Keywords: []string{
			"legal", "law", "court", "judge", "attorney", "lawyer", "lawsuit",
			"litigation", "contract", "regulation", "compliance", "statute",
			"patent", "trademark", "copyright", "intellectual property",
			"privacy", "gdpr", "hipaa", "terms of service", "eula",
		},
		Hunters:     []string{"legal"},
		Description: "Legal research, regulations, compliance",
	},
	{
		Name: "economics",
		Keywords: []string{
			"economics", "economy", "market", "trade", "gdp", "inflation",
			"employment", "unemployment", "labor", "workforce", "salary",
			"wage", "investment", "stock", "bond", "finance", "banking",
			"monetary", "fiscal", "tariff", "export", "import",
		},
		Hunters:     []string{"economics"},
		Description: "Economic data, labor statistics, market research",
	},
	{
		Name: "technology",
		Keywords: []string{
			"software", "programming", "code", "developer", "api", "framework",
			"library", "open source", "github", "git", "cloud", "server",
			"database", "frontend", "backend", "devops", "ci/cd", "testing",
			"security", "encryption", "authentication", "machine learning",
			"ai", "artificial intelligence", "llm", "neural network",
		},
		Hunters:     []string{"github-scout", "hackernews", "arxiv"},
		Description: "Software development, open source, technology trends",
	},
	{
		Name: "academic",
		Keywords: []string{
			"research", "study", "paper", "journal", "publication", "peer review",
			"citation", "thesis", "dissertation", "university", "professor",
			"academic", "scholar", "science", "experiment", "methodology",
		},
		Hunters:     []string{"openalex", "arxiv"},
		Description: "General academic research across disciplines",
	},
	{
		Name:        "general",
		Keywords:    []string{},
		Hunters:     []string{"wiki", "openalex"},
		Description: "General knowledge and reference",
	},
}

// FindDomain returns the domain that best matches the given text.
// Returns the "general" domain if no specific match is found.
func FindDomain(text string) Domain {
	scores := make(map[string]int)

	textLower := toLower(text)

	for _, domain := range Domains {
		if domain.Name == "general" {
			continue // Skip general for scoring
		}

		score := 0
		for _, keyword := range domain.Keywords {
			if containsWord(textLower, keyword) {
				score++
			}
		}
		if score > 0 {
			scores[domain.Name] = score
		}
	}

	// Find best match
	bestDomain := ""
	bestScore := 0
	for name, score := range scores {
		if score > bestScore {
			bestScore = score
			bestDomain = name
		}
	}

	// Return matching domain or general
	if bestDomain != "" {
		for _, d := range Domains {
			if d.Name == bestDomain {
				return d
			}
		}
	}

	// Return general domain
	for _, d := range Domains {
		if d.Name == "general" {
			return d
		}
	}

	return Domain{Name: "general", Hunters: []string{"wiki", "openalex"}}
}

// GetDomainByName returns a domain by its name.
func GetDomainByName(name string) (Domain, bool) {
	for _, d := range Domains {
		if d.Name == name {
			return d, true
		}
	}
	return Domain{}, false
}

// Helper functions

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

func containsWord(text, word string) bool {
	// Simple substring matching for now
	// Could be improved with word boundary detection
	return contains(text, word)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
