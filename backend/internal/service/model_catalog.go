package service

import (
	"context"
	"errors"
	"strings"
	"time"
)

const ModelCatalogPricingUnit = "per_1m_tokens"

var ErrModelCatalogModelNotFound = errors.New("model catalog model not found")

type ModelCatalogRepository interface {
	ListModelCatalogPricingOverrides(ctx context.Context) ([]ModelCatalogPricingOverride, error)
	GetModelCatalogPricingOverride(ctx context.Context, modelID string) (*ModelCatalogPricingOverride, error)
	UpsertModelCatalogPricingOverride(ctx context.Context, override ModelCatalogPricingOverride) error
	DeleteModelCatalogPricingOverrides(ctx context.Context) error
}

type ModelCatalogService struct {
	repo ModelCatalogRepository
}

type ModelCatalogProvider struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	DisplayName string              `json:"display_name"`
	LogoKey     string              `json:"logo_key"`
	Models      []ModelCatalogModel `json:"models"`
}

type ModelCatalogModel struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Description   string              `json:"description"`
	Tags          []string            `json:"tags"`
	ContextTokens int                 `json:"context_tokens"`
	OutputTokens  int                 `json:"output_tokens"`
	Pricing       ModelCatalogPricing `json:"pricing"`
	PricingSource string              `json:"pricing_source"`
}

type ModelCatalogPricing struct {
	Unit          string   `json:"unit"`
	Currency      string   `json:"currency"`
	DisplaySymbol string   `json:"display_symbol"`
	Input         *float64 `json:"input"`
	Output        *float64 `json:"output"`
	CacheWrite    *float64 `json:"cache_write"`
	CacheRead     *float64 `json:"cache_read"`
}

type ModelCatalogPricingOverride struct {
	ProviderID string
	ModelID    string
	Input      *float64
	Output     *float64
	CacheWrite *float64
	CacheRead  *float64
	UpdatedBy  *int64
	UpdatedAt  time.Time
}

type ModelCatalogPricingUpdate struct {
	Input      *float64
	Output     *float64
	CacheWrite *float64
	CacheRead  *float64
	UpdatedBy  *int64
}

func NewModelCatalogService(repo ModelCatalogRepository) *ModelCatalogService {
	return &ModelCatalogService{repo: repo}
}

func (s *ModelCatalogService) ListCatalog(ctx context.Context) ([]ModelCatalogProvider, error) {
	providers := defaultModelCatalogProviders()
	if s == nil || s.repo == nil {
		return providers, nil
	}

	overrides, err := s.repo.ListModelCatalogPricingOverrides(ctx)
	if err != nil {
		return nil, err
	}

	byModel := make(map[string]ModelCatalogPricingOverride, len(overrides))
	for _, override := range overrides {
		byModel[normalizeCatalogModelID(override.ModelID)] = override
	}

	for providerIdx := range providers {
		for modelIdx := range providers[providerIdx].Models {
			model := &providers[providerIdx].Models[modelIdx]
			override, ok := byModel[normalizeCatalogModelID(model.ID)]
			if !ok {
				continue
			}
			applyCatalogPricingOverride(model, override)
		}
	}

	return providers, nil
}

func (s *ModelCatalogService) UpdatePricing(ctx context.Context, modelID string, update ModelCatalogPricingUpdate) (*ModelCatalogModel, error) {
	providerID, model, ok := findDefaultCatalogModel(modelID)
	if !ok {
		return nil, ErrModelCatalogModelNotFound
	}
	if s == nil || s.repo == nil {
		return nil, errors.New("model catalog repository is not configured")
	}

	override := ModelCatalogPricingOverride{
		ProviderID: providerID,
		ModelID:    model.ID,
		Input:      update.Input,
		Output:     update.Output,
		CacheWrite: update.CacheWrite,
		CacheRead:  update.CacheRead,
		UpdatedBy:  update.UpdatedBy,
	}
	if err := s.repo.UpsertModelCatalogPricingOverride(ctx, override); err != nil {
		return nil, err
	}

	applyCatalogPricingOverride(&model, override)
	return &model, nil
}

func (s *ModelCatalogService) ResetPricingOverrides(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return nil
	}
	return s.repo.DeleteModelCatalogPricingOverrides(ctx)
}

func (s *ModelCatalogService) ResolveModelPricing(ctx context.Context, model string) (*ModelPricing, bool, error) {
	_, defaultModel, ok := findDefaultCatalogModel(model)
	if !ok || s == nil || s.repo == nil {
		return nil, false, nil
	}

	override, err := s.repo.GetModelCatalogPricingOverride(ctx, defaultModel.ID)
	if err != nil {
		return nil, false, err
	}
	if override == nil || !override.hasAnyPrice() {
		return nil, false, nil
	}

	applyCatalogPricingOverride(&defaultModel, *override)
	return catalogPricingToModelPricing(defaultModel.Pricing), true, nil
}

func defaultModelCatalogProviders() []ModelCatalogProvider {
	return []ModelCatalogProvider{
		{
			ID:          "anthropic",
			Name:        "Anthropic",
			DisplayName: "Anthropic",
			LogoKey:     "anthropic",
			Models: []ModelCatalogModel{
				catalogModel("claude-opus-4-7", "Claude Opus 4.7", "Anthropic Opus flagship model for deep reasoning and complex agent work.", []string{"chat", "reasoning", "tools", "vision"}, 1000000, 128000, modelCatalogPricingOf(5, 25, 6.25, 0.5)),
				catalogModel("claude-opus-4-6-thinking", "Claude Opus 4.6 / thinking", "Opus 4.6 with extended thinking for difficult planning, coding and analysis tasks.", []string{"chat", "reasoning", "thinking", "tools"}, 1000000, 128000, modelCatalogPricingOf(5, 25, 6.25, 0.5)),
				catalogModel("claude-sonnet-4-6", "Claude Sonnet 4.6", "Balanced Claude model for everyday coding, agents and long-context workflows.", []string{"chat", "coding", "tools", "vision"}, 1000000, 64000, modelCatalogPricingOf(3, 15, 3.75, 0.3)),
				catalogModel("claude-haiku-4-5", "Claude Haiku 4.5", "Fast Claude model for lightweight chat, classification and quick tool calls.", []string{"chat", "fast", "tools", "vision"}, 200000, 64000, modelCatalogPricingOf(1, 5, 1.25, 0.1)),
			},
		},
		{
			ID:          "openai",
			Name:        "OpenAI",
			DisplayName: "OpenAI",
			LogoKey:     "openai",
			Models: []ModelCatalogModel{
				catalogModel("gpt-5.5", "GPT-5.5", "OpenAI flagship model for complex coding, research and real-world work.", []string{"chat", "reasoning", "coding", "tools"}, 1050000, 128000, modelCatalogPricingOf(5, 30, 5, 0.5)),
				catalogModel("gpt-5.4", "GPT-5.4", "Strong general-purpose model for everyday coding and product workflows.", []string{"chat", "coding", "tools"}, 1050000, 128000, modelCatalogPricingOf(2.5, 15, 2.5, 0.25)),
				catalogModel("gpt-5.4-mini", "GPT-5.4 Mini", "Lower-cost OpenAI model for high-volume chat, routing and lightweight analysis.", []string{"chat", "fast", "tools"}, 400000, 128000, modelCatalogPricingOf(0.75, 4.5, 0.75, 0.075)),
				catalogModel("gpt-5.2", "GPT-5.2", "Professional work model with broad reasoning and long-running agent support.", []string{"chat", "reasoning", "coding"}, 272000, 128000, modelCatalogPricingOf(1.75, 14, 1.75, 0.175)),
			},
		},
		{
			ID:          "google",
			Name:        "Google",
			DisplayName: "Google",
			LogoKey:     "gemini",
			Models: []ModelCatalogModel{
				catalogModel("gemini-3.1-pro-high", "Gemini 3.1 Pro High", "Google Gemini Pro tier for long-context reasoning and multimodal work.", []string{"chat", "reasoning", "vision", "tools"}, 1048576, 65536, modelCatalogPricingOf(2, 12, 2, 0.2)),
				catalogModel("gemini-3.1-pro-low", "Gemini 3.1 Pro Low", "Google Gemini Pro low-latency routing profile for balanced workloads.", []string{"chat", "reasoning", "vision"}, 1048576, 65536, modelCatalogPricingOf(2, 12, 2, 0.2)),
				catalogModel("gemini-3-flash-preview", "Gemini 3 Flash Preview", "Fast Gemini 3 family model for responsive multimodal requests.", []string{"chat", "fast", "vision", "preview"}, 1048576, 65535, modelCatalogPricingOf(0.5, 3, 0.5, 0.05)),
				catalogModel("gemini-2.5-pro", "Gemini 2.5 Pro", "Gemini Pro model for strong reasoning, code and long-context analysis.", []string{"chat", "reasoning", "vision"}, 1048576, 65535, modelCatalogPricingOf(1.25, 10, 1.25, 0.125)),
				catalogModel("gemini-2.5-flash", "Gemini 2.5 Flash", "Efficient Gemini model for high-volume chat, extraction and tool workflows.", []string{"chat", "fast", "vision"}, 1048576, 65535, modelCatalogPricingOf(0.3, 2.5, 0.3, 0.03)),
				catalogModel("gemini-2.5-flash-lite", "Gemini 2.5 Flash Lite", "Lowest-cost Gemini option for simple and latency-sensitive tasks.", []string{"chat", "fast"}, 1048576, 65535, modelCatalogPricingOf(0.1, 0.4, 0.1, 0.01)),
			},
		},
		{
			ID:          "deepseek",
			Name:        "DeepSeek",
			DisplayName: "DeepSeek",
			LogoKey:     "deepseek",
			Models: []ModelCatalogModel{
				catalogModel("deepseek-v4-flash", "DeepSeek V4 Flash", "DeepSeek V4 fast profile for general chat, coding and extraction.", []string{"chat", "coding", "fast"}, 131072, 8192, modelCatalogPricingOf(1.15, 2.5, 1.15, 0.03)),
				catalogModel("deepseek-v4-pro", "DeepSeek V4 Pro", "DeepSeek V4 reasoning profile for complex problem solving and coding tasks.", []string{"chat", "reasoning", "coding"}, 131072, 65536, modelCatalogPricingOf(4, 7.5, 4, 0.035)),
			},
		},
		{
			ID:          "moonshot",
			Name:        "Moonshot",
			DisplayName: "Moonshot",
			LogoKey:     "kimi",
			Models: []ModelCatalogModel{
				catalogModel("kimi-k2.6-full", "Kimi K2.6 Full", "Moonshot Kimi full-capability profile for agentic coding and complex chat.", []string{"chat", "coding", "tools"}, 1000000, 64000, modelCatalogPricingOf(4, 16, 4, 0.8)),
				catalogModel("kimi-k2.6-tools-search", "Kimi K2.6 Tools + Search", "Kimi K2.6 profile with tool use and search-oriented workflows.", []string{"chat", "tools", "search"}, 1000000, 64000, modelCatalogPricingOf(4, 16, 4, 0.8)),
				catalogModel("kimi-k2.6-multimodal", "Kimi K2.6 Multimodal", "Kimi multimodal profile for text and visual understanding.", []string{"chat", "vision", "tools"}, 1000000, 64000, modelCatalogPricingOf(4, 16, 4, 0.8)),
				catalogModel("kimi-for-coding", "Kimi for Coding", "Moonshot coding-focused model for repository reading and code generation.", []string{"chat", "coding", "tools"}, 256000, 32000, modelCatalogPricingOf(2, 8, 2, 0.4)),
			},
		},
		{
			ID:          "mimo",
			Name:        "MiMo",
			DisplayName: "MiMo",
			LogoKey:     "mimo",
			Models: []ModelCatalogModel{
				catalogModel("mimo-v2.5-pro", "MiMo 2.5 Pro", "MiMo flagship profile for OpenAI-compatible and Anthropic-compatible requests.", []string{"chat", "reasoning", "tools"}, 256000, 64000, modelCatalogPricingOf(4, 16, 4, 0.8)),
				catalogModel("mimo-v2.5", "MiMo 2.5", "Balanced MiMo model for daily chat, coding and API conversion workflows.", []string{"chat", "coding", "tools"}, 128000, 32000, modelCatalogPricingOf(2, 8, 2, 0.4)),
				catalogModel("mimo-v2-omni", "MiMo Omni", "MiMo multimodal model for mixed media understanding and chat.", []string{"chat", "vision", "audio"}, 128000, 32000, modelCatalogPricingOf(2, 8, 2, 0.4)),
				catalogModel("mimo-v2-flash", "MiMo Flash", "Fast MiMo model for lightweight prompts and low-latency routing.", []string{"chat", "fast"}, 64000, 16000, modelCatalogPricingOf(0.8, 2.4, 0.8, 0.16)),
			},
		},
	}
}

func catalogModel(id, name, description string, tags []string, contextTokens, outputTokens int, pricing ModelCatalogPricing) ModelCatalogModel {
	return ModelCatalogModel{
		ID:            id,
		Name:          name,
		Description:   description,
		Tags:          append([]string(nil), tags...),
		ContextTokens: contextTokens,
		OutputTokens:  outputTokens,
		Pricing:       pricing,
		PricingSource: "official",
	}
}

func modelCatalogPricingOf(input, output any, cacheWrite any, cacheRead any) ModelCatalogPricing {
	return ModelCatalogPricing{
		Unit:          ModelCatalogPricingUnit,
		Currency:      "CNY_DISPLAY",
		DisplaySymbol: "\uffe5",
		Input:         pricePtr(input),
		Output:        pricePtr(output),
		CacheWrite:    pricePtr(cacheWrite),
		CacheRead:     pricePtr(cacheRead),
	}
}

func pricePtr(value any) *float64 {
	switch v := value.(type) {
	case nil:
		return nil
	case int:
		out := float64(v)
		return &out
	case float64:
		return &v
	default:
		return nil
	}
}

func applyCatalogPricingOverride(model *ModelCatalogModel, override ModelCatalogPricingOverride) {
	if model == nil {
		return
	}
	if override.Input != nil {
		model.Pricing.Input = cloneFloat64Ptr(override.Input)
	}
	if override.Output != nil {
		model.Pricing.Output = cloneFloat64Ptr(override.Output)
	}
	if override.CacheWrite != nil {
		model.Pricing.CacheWrite = cloneFloat64Ptr(override.CacheWrite)
	}
	if override.CacheRead != nil {
		model.Pricing.CacheRead = cloneFloat64Ptr(override.CacheRead)
	}
	if override.hasAnyPrice() {
		model.PricingSource = "override"
	}
}

func (o ModelCatalogPricingOverride) hasAnyPrice() bool {
	return o.Input != nil || o.Output != nil || o.CacheWrite != nil || o.CacheRead != nil
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func findDefaultCatalogModel(model string) (string, ModelCatalogModel, bool) {
	needle := normalizeCatalogModelID(model)
	if needle == "" {
		return "", ModelCatalogModel{}, false
	}
	for _, provider := range defaultModelCatalogProviders() {
		for _, candidate := range provider.Models {
			if normalizeCatalogModelID(candidate.ID) == needle || normalizeCatalogModelID(candidate.Name) == needle {
				return provider.ID, candidate, true
			}
		}
	}
	return "", ModelCatalogModel{}, false
}

func normalizeCatalogModelID(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

func catalogPricingToModelPricing(pricing ModelCatalogPricing) *ModelPricing {
	out := &ModelPricing{}
	if pricing.Input != nil {
		price := perMillionToPerToken(*pricing.Input)
		out.InputPricePerToken = price
		out.InputPricePerTokenPriority = price
	}
	if pricing.Output != nil {
		price := perMillionToPerToken(*pricing.Output)
		out.OutputPricePerToken = price
		out.OutputPricePerTokenPriority = price
	}
	if pricing.CacheWrite != nil {
		price := perMillionToPerToken(*pricing.CacheWrite)
		out.CacheCreationPricePerToken = price
		out.CacheCreation5mPrice = price
		out.CacheCreation1hPrice = price
		out.SupportsCacheBreakdown = true
	}
	if pricing.CacheRead != nil {
		price := perMillionToPerToken(*pricing.CacheRead)
		out.CacheReadPricePerToken = price
		out.CacheReadPricePerTokenPriority = price
		out.SupportsCacheBreakdown = true
	}
	return out
}

func perMillionToPerToken(price float64) float64 {
	return price / 1_000_000
}
