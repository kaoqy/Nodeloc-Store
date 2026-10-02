package infrastructure

import (
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
)

// TestKnowledgeScorePrefersTitleAndKeywords pins the ranking weights: a term in
// the title must outrank the same term buried in the body, otherwise the AI
// would quote a passing mention over the article written for the question.
func TestKnowledgeScorePrefersTitleAndKeywords(t *testing.T) {
	terms := splitTerms("退款 规则")
	title := models.AIKnowledge{Title: "退款规则", Content: "其他内容"}
	body := models.AIKnowledge{Title: "购买流程", Content: "这里顺便提到退款两个字"}
	if knowledgeScore(title, terms) <= knowledgeScore(body, terms) {
		t.Fatalf("title hit scored %v, body hit scored %v", knowledgeScore(title, terms), knowledgeScore(body, terms))
	}
}

// TestSearchTermsSplitsPunctuation keeps the query splitter honest: Chinese
// punctuation is a separator, not part of a search term.
func TestSearchTermsSplitsPunctuation(t *testing.T) {
	terms := splitTerms("退款，怎么申请？")
	if len(terms) != 2 {
		t.Fatalf("terms = %v, want 2", terms)
	}
	for _, term := range terms {
		if term == "" {
			t.Fatalf("empty term in %v", terms)
		}
	}
}
