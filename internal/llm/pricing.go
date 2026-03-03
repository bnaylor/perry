package llm

import (
	"fmt"
	"strings"
)

// ModelPrice defines the pricing for 1M tokens.
type ModelPrice struct {
	InputPrice  float64 // USD per 1M tokens
	OutputPrice float64 // USD per 1M tokens
}

// PricingTable maps model identifiers to their 2026 price structures.
// Rates as of March 3, 2026.
var PricingTable = map[string]ModelPrice{
	// Gemini 3.1 Family (Google AI Studio / Vertex AI)
	"gemini-3.1-pro-preview":  {InputPrice: 2.00, OutputPrice: 12.00},
	"gemini-3.1-pro":          {InputPrice: 2.00, OutputPrice: 12.00},
	"gemini-3-flash-preview":  {InputPrice: 0.50, OutputPrice: 3.00},
	"gemini-3-flash":          {InputPrice: 0.50, OutputPrice: 3.00},
	"gemini-3.1-flash-lite":   {InputPrice: 0.05, OutputPrice: 0.25}, // Estimated Lite pricing
	"gemini-2.0-flash":        {InputPrice: 0.10, OutputPrice: 0.40}, // Legacy pricing

	// Claude 3.7 Family (Anthropic)
	"claude-3-7-opus-20260219":   {InputPrice: 15.00, OutputPrice: 75.00},
	"claude-3-7-sonnet-20260219": {InputPrice: 3.00, OutputPrice: 15.00},
	"claude-3-7-haiku-20260303":  {InputPrice: 0.25, OutputPrice: 1.25},
}

// EstimateCost calculates the projected cost for a given model and token counts.
func EstimateCost(model string, inputTokens, outputTokens int) (float64, error) {
	// Simple prefix matching for model versions
	var price ModelPrice
	var found bool

	for m, p := range PricingTable {
		if strings.HasPrefix(model, m) {
			price = p
			found = true
			break
		}
	}

	if !found {
		return 0, fmt.Errorf("pricing not found for model: %s", model)
	}

	inputCost := (float64(inputTokens) / 1000000.0) * price.InputPrice
	outputCost := (float64(outputTokens) / 1000000.0) * price.OutputPrice

	return inputCost + outputCost, nil
}
