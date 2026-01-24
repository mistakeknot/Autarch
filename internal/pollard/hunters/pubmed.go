// Package hunters provides research agent implementations for Pollard.
package hunters

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// PubMedHunter searches biomedical literature from PubMed/NCBI.
// PubMed indexes 37M+ biomedical citations.
type PubMedHunter struct {
	client      *http.Client
	rateLimiter *RateLimiter
	apiKey      string // Optional NCBI API key for faster rate limits
}

// NewPubMedHunter creates a new PubMed research hunter.
func NewPubMedHunter() *PubMedHunter {
	apiKey := os.Getenv("NCBI_API_KEY")
	// Rate limit: 3 req/s without key, 10 req/s with key
	authenticated := apiKey != ""
	rateLimit := 3
	if authenticated {
		rateLimit = 10
	}
	return &PubMedHunter{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		rateLimiter: NewRateLimiter(rateLimit, time.Second, authenticated),
		apiKey:      apiKey,
	}
}

// Name returns the hunter's identifier.
func (h *PubMedHunter) Name() string {
	return "pubmed"
}

// Hunt performs the research collection from PubMed.
func (h *PubMedHunter) Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error) {
	result := &HuntResult{
		HunterName: h.Name(),
		StartedAt:  time.Now(),
	}

	maxResults := cfg.MaxResults
	if maxResults <= 0 {
		maxResults = 50
	}

	var allArticles []PubMedArticle
	var errors []error

	for _, query := range cfg.Queries {
		select {
		case <-ctx.Done():
			result.Errors = append(result.Errors, ctx.Err())
			result.CompletedAt = time.Now()
			return result, ctx.Err()
		default:
		}

		// Wait for rate limiter
		if err := h.rateLimiter.Wait(ctx); err != nil {
			errors = append(errors, fmt.Errorf("rate limit wait for query %q: %w", query, err))
			continue
		}

		articles, err := h.searchPubMed(ctx, query, maxResults)
		if err != nil {
			errors = append(errors, fmt.Errorf("search %q: %w", query, err))
			continue
		}

		allArticles = append(allArticles, articles...)
	}

	// Deduplicate articles by PMID
	seen := make(map[string]bool)
	uniqueArticles := make([]PubMedArticle, 0, len(allArticles))
	for _, a := range allArticles {
		if !seen[a.PMID] {
			seen[a.PMID] = true
			uniqueArticles = append(uniqueArticles, a)
		}
	}

	// Save results
	if len(uniqueArticles) > 0 {
		outputFile, err := h.saveResults(cfg, uniqueArticles, cfg.Queries)
		if err != nil {
			errors = append(errors, fmt.Errorf("save results: %w", err))
		} else {
			result.OutputFiles = append(result.OutputFiles, outputFile)
		}
	}

	result.SourcesCollected = len(uniqueArticles)
	result.Errors = errors
	result.CompletedAt = time.Now()

	return result, nil
}

// searchPubMed queries the NCBI E-utilities API for articles matching the query.
func (h *PubMedHunter) searchPubMed(ctx context.Context, query string, maxResults int) ([]PubMedArticle, error) {
	// Step 1: ESearch to get PMIDs
	pmids, err := h.esearch(ctx, query, maxResults)
	if err != nil {
		return nil, fmt.Errorf("esearch: %w", err)
	}

	if len(pmids) == 0 {
		return nil, nil
	}

	// Step 2: EFetch to get article details
	return h.efetch(ctx, pmids, query)
}

// esearch performs an ESearch query to get PMIDs.
func (h *PubMedHunter) esearch(ctx context.Context, query string, maxResults int) ([]string, error) {
	apiURL := fmt.Sprintf(
		"https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi?db=pubmed&term=%s&retmax=%d&retmode=xml&sort=relevance",
		url.QueryEscape(query),
		maxResults,
	)

	if h.apiKey != "" {
		apiURL += "&api_key=" + h.apiKey
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ESearch returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse ESearch response
	var esearchResult struct {
		XMLName xml.Name `xml:"eSearchResult"`
		IDList  struct {
			IDs []string `xml:"Id"`
		} `xml:"IdList"`
	}

	if err := xml.Unmarshal(body, &esearchResult); err != nil {
		return nil, fmt.Errorf("parse esearch XML: %w", err)
	}

	return esearchResult.IDList.IDs, nil
}

// efetch fetches article details for the given PMIDs.
func (h *PubMedHunter) efetch(ctx context.Context, pmids []string, originalQuery string) ([]PubMedArticle, error) {
	// Wait for rate limiter between requests
	if err := h.rateLimiter.Wait(ctx); err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf(
		"https://eutils.ncbi.nlm.nih.gov/entrez/eutils/efetch.fcgi?db=pubmed&id=%s&retmode=xml",
		strings.Join(pmids, ","),
	)

	if h.apiKey != "" {
		apiURL += "&api_key=" + h.apiKey
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("EFetch returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parsePubMedResponse(body, originalQuery)
}

// PubMed XML structures
type pubmedArticleSet struct {
	XMLName  xml.Name        `xml:"PubmedArticleSet"`
	Articles []pubmedArticle `xml:"PubmedArticle"`
}

type pubmedArticle struct {
	MedlineCitation struct {
		PMID struct {
			Value string `xml:",chardata"`
		} `xml:"PMID"`
		Article struct {
			ArticleTitle string `xml:"ArticleTitle"`
			Abstract     struct {
				AbstractText []struct {
					Label string `xml:"Label,attr"`
					Text  string `xml:",chardata"`
				} `xml:"AbstractText"`
			} `xml:"Abstract"`
			AuthorList struct {
				Authors []struct {
					LastName string `xml:"LastName"`
					ForeName string `xml:"ForeName"`
				} `xml:"Author"`
			} `xml:"AuthorList"`
			Journal struct {
				Title string `xml:"Title"`
				PubDate struct {
					Year  string `xml:"Year"`
					Month string `xml:"Month"`
					Day   string `xml:"Day"`
				} `xml:"JournalIssue>PubDate"`
			} `xml:"Journal"`
		} `xml:"Article"`
		MeshHeadingList struct {
			MeshHeadings []struct {
				DescriptorName struct {
					Name string `xml:",chardata"`
				} `xml:"DescriptorName"`
			} `xml:"MeshHeading"`
		} `xml:"MeshHeadingList"`
		KeywordList struct {
			Keywords []struct {
				Keyword string `xml:",chardata"`
			} `xml:"Keyword"`
		} `xml:"KeywordList"`
	} `xml:"MedlineCitation"`
	PubmedData struct {
		ArticleIDList struct {
			ArticleIDs []struct {
				IDType string `xml:"IdType,attr"`
				ID     string `xml:",chardata"`
			} `xml:"ArticleId"`
		} `xml:"ArticleIdList"`
	} `xml:"PubmedData"`
}

// parsePubMedResponse parses the XML response from PubMed EFetch.
func parsePubMedResponse(data []byte, originalQuery string) ([]PubMedArticle, error) {
	var articleSet pubmedArticleSet
	if err := xml.Unmarshal(data, &articleSet); err != nil {
		return nil, fmt.Errorf("parse XML: %w", err)
	}

	articles := make([]PubMedArticle, 0, len(articleSet.Articles))
	for _, a := range articleSet.Articles {
		// Extract authors
		var authors []string
		for _, author := range a.MedlineCitation.Article.AuthorList.Authors {
			name := strings.TrimSpace(author.ForeName + " " + author.LastName)
			if name != "" {
				authors = append(authors, name)
			}
		}

		// Extract abstract (may have multiple parts)
		var abstractParts []string
		for _, part := range a.MedlineCitation.Article.Abstract.AbstractText {
			text := strings.TrimSpace(part.Text)
			if text != "" {
				if part.Label != "" {
					text = part.Label + ": " + text
				}
				abstractParts = append(abstractParts, text)
			}
		}
		abstract := strings.Join(abstractParts, " ")

		// Extract MeSH terms
		var meshTerms []string
		for _, mesh := range a.MedlineCitation.MeshHeadingList.MeshHeadings {
			if mesh.DescriptorName.Name != "" {
				meshTerms = append(meshTerms, mesh.DescriptorName.Name)
			}
		}

		// Extract keywords
		var keywords []string
		for _, kw := range a.MedlineCitation.KeywordList.Keywords {
			if kw.Keyword != "" {
				keywords = append(keywords, kw.Keyword)
			}
		}

		// Parse publication date
		pubDate := a.MedlineCitation.Article.Journal.PubDate
		var publishedAt string
		if pubDate.Year != "" {
			publishedAt = pubDate.Year
			if pubDate.Month != "" {
				publishedAt += "-" + normalizeMonth(pubDate.Month)
				if pubDate.Day != "" {
					publishedAt += "-" + fmt.Sprintf("%02s", pubDate.Day)
				}
			}
		}

		// Find DOI in article IDs
		var doi string
		for _, id := range a.PubmedData.ArticleIDList.ArticleIDs {
			if id.IDType == "doi" {
				doi = id.ID
				break
			}
		}

		pmid := a.MedlineCitation.PMID.Value
		article := PubMedArticle{
			PMID:        pmid,
			Title:       a.MedlineCitation.Article.ArticleTitle,
			Authors:     authors,
			Abstract:    abstract,
			Journal:     a.MedlineCitation.Article.Journal.Title,
			PublishedAt: publishedAt,
			MeSHTerms:   meshTerms,
			Keywords:    keywords,
			DOI:         doi,
			URL:         fmt.Sprintf("https://pubmed.ncbi.nlm.nih.gov/%s/", pmid),
			Relevance:   assessPubMedRelevance(a.MedlineCitation.Article.ArticleTitle, abstract, originalQuery),
		}
		articles = append(articles, article)
	}

	return articles, nil
}

// normalizeMonth converts month name or number to two-digit number.
func normalizeMonth(month string) string {
	months := map[string]string{
		"Jan": "01", "Feb": "02", "Mar": "03", "Apr": "04",
		"May": "05", "Jun": "06", "Jul": "07", "Aug": "08",
		"Sep": "09", "Oct": "10", "Nov": "11", "Dec": "12",
		"January": "01", "February": "02", "March": "03", "April": "04",
		"June": "06", "July": "07", "August": "08",
		"September": "09", "October": "10", "November": "11", "December": "12",
	}
	if m, ok := months[month]; ok {
		return m
	}
	// Try to use as-is if it's already numeric
	if len(month) <= 2 {
		return fmt.Sprintf("%02s", month)
	}
	return "01"
}

// assessPubMedRelevance determines the relevance level of an article.
func assessPubMedRelevance(title, abstract, query string) string {
	queryLower := strings.ToLower(query)
	titleLower := strings.ToLower(title)
	abstractLower := strings.ToLower(abstract)

	// Query words in title is high signal
	queryWords := strings.Fields(queryLower)
	titleMatches := 0
	for _, word := range queryWords {
		if len(word) > 3 && strings.Contains(titleLower, word) {
			titleMatches++
		}
	}

	if titleMatches >= 2 || strings.Contains(titleLower, queryLower) {
		return "high"
	}

	// Good abstract match
	abstractMatches := 0
	for _, word := range queryWords {
		if len(word) > 3 && strings.Contains(abstractLower, word) {
			abstractMatches++
		}
	}

	if abstractMatches >= 3 || titleMatches >= 1 {
		return "medium"
	}

	return "low"
}

// PubMedArticle represents an article in the output YAML format.
type PubMedArticle struct {
	PMID        string   `yaml:"pmid"`
	DOI         string   `yaml:"doi,omitempty"`
	Title       string   `yaml:"title"`
	Authors     []string `yaml:"authors"`
	Abstract    string   `yaml:"abstract,omitempty"`
	Journal     string   `yaml:"journal"`
	PublishedAt string   `yaml:"published_at"`
	MeSHTerms   []string `yaml:"mesh_terms,omitempty"`
	Keywords    []string `yaml:"keywords,omitempty"`
	URL         string   `yaml:"url"`
	Relevance   string   `yaml:"relevance"`
}

// PubMedOutput represents the complete output YAML structure.
type PubMedOutput struct {
	Query       string          `yaml:"query"`
	CollectedAt time.Time       `yaml:"collected_at"`
	Articles    []PubMedArticle `yaml:"articles"`
}

// saveResults saves the collected articles to a YAML file.
func (h *PubMedHunter) saveResults(cfg HunterConfig, articles []PubMedArticle, queries []string) (string, error) {
	// Determine output directory
	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir = "sources/pubmed"
	}

	// Ensure the directory exists
	fullOutputDir := filepath.Join(cfg.ProjectPath, ".pollard", outputDir)
	if err := os.MkdirAll(fullOutputDir, 0755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	// Generate filename with date
	filename := fmt.Sprintf("%s-pubmed.yaml", time.Now().Format("2006-01-02"))
	fullPath := filepath.Join(fullOutputDir, filename)

	// Create output structure
	queryStr := strings.Join(queries, ", ")
	output := PubMedOutput{
		Query:       queryStr,
		CollectedAt: time.Now().UTC(),
		Articles:    articles,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(&output)
	if err != nil {
		return "", fmt.Errorf("marshal YAML: %w", err)
	}

	// Write file
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return fullPath, nil
}
