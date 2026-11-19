package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"skill-progress-tracker/backend-go/internal/config"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/models"
)

const (
	MaxTreeNodes = 2000
	MaxTreeDepth = 20
	MaxNameLen   = 500

	TotalGenerationBudget = 46 * time.Second
	PerAttemptTimeout     = 22 * time.Second
	GeminiRestBase        = "https://generativelanguage.googleapis.com/v1beta/models"
)

var retiredModelMap = map[string]string{
	"":                      "gemini-3.8-flash",
	"gemini-1.5-flash":      "gemini-3.8-flash",
	"gemini-1.5-pro":        "gemini-3.8-flash",
	"gemini-2.0-flash":      "gemini-3.8-flash",
	"gemini-2.0-flash-lite": "gemini-3.5-flash-lite",
	"gemini-2.5-flash":      "gemini-3.8-flash",
	"gemini-2.5-flash-lite": "gemini-3.5-flash-lite",
}

var depthDescriptions = map[string]string{
	"easy": "Basic fundamentals only. Include definitions and introductory concepts. " +
		"Keep subtopics limited. Focus on essential beginner-level topics. " +
		"Aim for roughly 10-20 topics total.",
	"medium": "Cover fundamentals and commonly used important concepts. " +
		"Include relevant subtopics with moderate coverage. " +
		"Aim for roughly 20-40 topics total.",
	"intermediate": "Detailed concept coverage including important and moderately advanced concepts. " +
		"Include more subtopics and sub-subtopics. Cover practical and technical concepts. " +
		"Appropriate depth for serious learning. Aim for roughly 40-70 topics total.",
	"comprehensive": "Complete A-to-Z coverage of the subject. Cover fundamentals through advanced concepts. " +
		"Include all important subtopics and sub-subtopics. Include related concepts that are " +
		"genuinely relevant. Provide technical depth and practical knowledge. " +
		"Do not miss major areas of the subject. Aim for roughly 65-110 topics total.",
}

var purposeDescriptions = map[string]string{
	"interview": "Focus on technical interview preparation. Emphasize frequently tested concepts, " +
		"important problem-solving areas, practical implementation topics, " +
		"and relevant advanced interview topics.",
	"quiz": "Focus on quiz and examination preparation. Emphasize important examinable concepts, " +
		"definitions, classifications, properties, fundamentals, and conceptual understanding.",
	"a_to_z": "Focus on learning the entire subject comprehensively. Cover fundamentals, " +
		"intermediate concepts, and advanced concepts. Include important related areas. " +
		"Organize concepts in a logical learning sequence.",
}

const SystemPrompt = `You are an expert curriculum designer and syllabus generator for a skill progress tracking application.

Your task is to generate a hierarchical syllabus for a given subject, organized as topics, subtopics, and sub-subtopics.

CRITICAL RULES:
1. Return ONLY valid, compact JSON. Absolutely no prose, conversational filler, markdown explanation, or text before/after the JSON.
2. The JSON must follow this exact structure:
{"skill_name":"Subject Name","topics":[{"name":"Topic Name","children":[{"name":"Subtopic Name"}]}]}
3. Every topic must have a "name" (non-empty string). Parent topics must have a "children" array of child topic objects; leaf topics may omit "children" or use "children": [].
4. Output compact JSON without unnecessary indentation or whitespace so generation is fast and complete.
5. Use logical hierarchical parent-child relationships. Child topics must be nested directly under their parent's "children" array.
6. Topic names must be concise and meaningful (2-7 words).
7. Do NOT include numbering or bullet prefixes in topic names (no "1", "1.1", "1.2.1", "A.", "-", etc.) - the application automatically numbers the hierarchy.
8. Do NOT include emojis or decorative symbols.
9. Do NOT include markdown headings, formatting, or bullet points in topic names.
10. Do NOT include explanatory sentences, definitions, or paragraphs inside topic names.
11. Do NOT include introductory messages ("Here is your syllabus") or concluding remarks.
12. Do NOT create duplicate or near-duplicate sibling topics.
13. Do NOT create empty topics.
14. Avoid unnecessary nesting beyond 4 levels.`

var (
	reDecimalPrefix = regexp.MustCompile(`^\s*\d+(?:\.\d+)*\.?\s*[-—:]?\s*`)
	reBulletPrefix  = regexp.MustCompile(`^[\s•\-*]+\s*`)
	reCodeFence     = regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(.*?)\\n?```")
	reTrailingComma = regexp.MustCompile(`,\s*([\]\}])`)
	reModelSuggest  = regexp.MustCompile(`models/([a-zA-Z0-9._-]+)`)
)

func compactTreeForPrompt(node map[string]any) map[string]any {
	name, _ := node["name"].(string)
	out := map[string]any{"name": strings.TrimSpace(name)}
	if kidsRaw, ok := node["children"].([]any); ok && len(kidsRaw) > 0 {
		var kids []map[string]any
		for _, c := range kidsRaw {
			if cm, ok := c.(map[string]any); ok {
				kids = append(kids, compactTreeForPrompt(cm))
			}
		}
		if len(kids) > 0 {
			out["children"] = kids
		}
	}
	return out
}

func BuildGenerationPrompt(subject, depth, purpose string) string {
	dDesc, ok := depthDescriptions[strings.ToLower(depth)]
	if !ok {
		dDesc = depthDescriptions["medium"]
	}
	pDesc, ok := purposeDescriptions[strings.ToLower(purpose)]
	if !ok {
		pDesc = purposeDescriptions["a_to_z"]
	}
	capDepth := depth
	if len(capDepth) > 0 {
		capDepth = strings.ToUpper(capDepth[:1]) + strings.ToLower(capDepth[1:])
	}
	titlePurpose := strings.Title(strings.ReplaceAll(strings.ToLower(purpose), "_", " "))
	return fmt.Sprintf(
		"Generate a syllabus for: %s\n\n"+
			"Syllabus Depth: %s\n%s\n\n"+
			"Learning Purpose: %s\n%s\n\n"+
			"Return the complete syllabus as a single compact JSON object matching the required schema with \"skill_name\" and \"topics\".",
		subject, capDepth, dDesc, titlePurpose, pDesc,
	)
}

func BuildRefinementPrompt(subject, depth, purpose string, currentTree map[string]any, instruction string) string {
	var compactTopics []map[string]any
	if rawTopics, ok := currentTree["topics"].([]any); ok {
		for _, t := range rawTopics {
			if tm, ok := t.(map[string]any); ok {
				compactTopics = append(compactTopics, compactTreeForPrompt(tm))
			}
		}
	}
	skillName, _ := currentTree["skill_name"].(string)
	if strings.TrimSpace(skillName) == "" {
		skillName = subject
	}
	compactPayload := map[string]any{
		"skill_name": skillName,
		"topics":     compactTopics,
	}
	treeJSON, _ := json.Marshal(compactPayload)

	capDepth := depth
	if len(capDepth) > 0 {
		capDepth = strings.ToUpper(capDepth[:1]) + strings.ToLower(capDepth[1:])
	}
	titlePurpose := strings.Title(strings.ReplaceAll(strings.ToLower(purpose), "_", " "))

	return fmt.Sprintf(
		"You are modifying an existing syllabus based on the user's specific natural-language instruction.\n\n"+
			"Subject: %s\n"+
			"Original Depth: %s\n"+
			"Original Purpose: %s\n\n"+
			"Current Syllabus Tree (may contain manual edits by the user):\n%s\n\n"+
			"User Instruction: %s\n\n"+
			"IMPORTANT REFINEMENT GUIDELINES:\n"+
			"1. Strictly apply the user's instruction to the current syllabus.\n"+
			"2. If the user refers to specific topics or numbers (e.g., 'Expand topic 2.1' or 'Remove topic 4'), identify that specific topic in the hierarchy and modify it.\n"+
			"3. Preserve all other unrelated topics and manual edits made in the current syllabus.\n"+
			"4. Return the COMPLETE updated syllabus as a valid compact JSON object with \"skill_name\" and \"topics\".\n"+
			"5. Do NOT include numbering prefixes in topic names.\n"+
			"6. Return ONLY the JSON object. No extra text.",
		subject, capDepth, titlePurpose, string(treeJSON), instruction,
	)
}

// RepairTruncatedJSON salvages a valid syllabus JSON object if the model output was truncated near the end.
func RepairTruncatedJSON(text string) map[string]any {
	start := strings.Index(text, "{")
	if start == -1 {
		return nil
	}
	fragment := text[start:]
	pos := len(fragment)
	attempts := 0

	for pos > 0 && attempts < 50 {
		lastBrace := strings.LastIndex(fragment[:pos], "}")
		if lastBrace == -1 {
			break
		}
		candidate := fragment[:lastBrace+1]
		var stack []byte
		inStr := false
		escape := false
		validScan := true

		for i := 0; i < len(candidate); i++ {
			ch := candidate[i]
			if escape {
				escape = false
				continue
			}
			if ch == '\\' {
				if inStr {
					escape = true
				}
				continue
			}
			if ch == '"' {
				inStr = !inStr
				continue
			}
			if inStr {
				continue
			}
			switch ch {
			case '{':
				stack = append(stack, '}')
			case '[':
				stack = append(stack, ']')
			case '}', ']':
				if len(stack) == 0 || stack[len(stack)-1] != ch {
					validScan = false
					break
				}
				stack = stack[:len(stack)-1]
			}
			if !validScan {
				break
			}
		}

		if validScan && !inStr {
			var rev strings.Builder
			for i := len(stack) - 1; i >= 0; i-- {
				rev.WriteByte(stack[i])
			}
			repaired := candidate + rev.String()
			repaired = reTrailingComma.ReplaceAllString(repaired, "$1")
			var parsed map[string]any
			if err := json.Unmarshal([]byte(repaired), &parsed); err == nil && parsed != nil {
				if topics, ok := parsed["topics"].([]any); ok && len(topics) > 0 {
					return parsed
				}
			}
		}
		pos = lastBrace
		attempts++
	}
	return nil
}

// ExtractJSON safely extracts and parses JSON from Gemini output with trailing comma and truncation tolerance.
func ExtractJSON(text string) (map[string]any, error) {
	cleaned := strings.TrimSpace(text)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(cleaned), &parsed); err == nil && parsed != nil {
		return parsed, nil
	}

	if m := reCodeFence.FindStringSubmatch(cleaned); len(m) > 1 {
		snippet := strings.TrimSpace(m[1])
		if err := json.Unmarshal([]byte(snippet), &parsed); err == nil && parsed != nil {
			return parsed, nil
		}
		cleaned = snippet
	}

	start := strings.Index(cleaned, "{")
	end := strings.LastIndex(cleaned, "}")
	if start != -1 && end > start {
		rawJSON := cleaned[start : end+1]
		if err := json.Unmarshal([]byte(rawJSON), &parsed); err == nil && parsed != nil {
			return parsed, nil
		}
		sanitized := reTrailingComma.ReplaceAllString(rawJSON, "$1")
		if err := json.Unmarshal([]byte(sanitized), &parsed); err == nil && parsed != nil {
			return parsed, nil
		}
	}

	if repaired := RepairTruncatedJSON(cleaned); repaired != nil {
		core.Logger.Warn("Repaired truncated JSON response from Gemini")
		return repaired, nil
	}

	return nil, core.NewAppError(502, "ai_invalid_response", "AI returned invalid JSON. Please try again.")
}

func SanitizeAIName(name string) string {
	trimmed := strings.TrimSpace(name)
	cleaned := reDecimalPrefix.ReplaceAllString(trimmed, "")
	cleaned = reBulletPrefix.ReplaceAllString(cleaned, "")
	if cleaned == "" {
		cleaned = trimmed
	}
	runes := []rune(cleaned)
	if len(runes) > MaxNameLen {
		cleaned = string(runes[:MaxNameLen])
	}
	return cleaned
}

func validateNode(node any, depth int) (*models.TopicNodeAI, error) {
	m, ok := node.(map[string]any)
	if !ok {
		return nil, nil
	}
	var rawName string
	for _, key := range []string{"name", "title", "topic"} {
		if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
			rawName = v
			break
		}
	}
	if strings.TrimSpace(rawName) == "" {
		return nil, nil
	}
	name := SanitizeAIName(rawName)
	if name == "" {
		return nil, nil
	}

	if depth > MaxTreeDepth {
		return nil, core.NewAppError(502, "ai_invalid_response", "AI response exceeds maximum nesting depth.")
	}

	var childrenRaw []any
	if c, ok := m["children"].([]any); ok {
		childrenRaw = c
	} else if s, ok := m["subtopics"].([]any); ok {
		childrenRaw = s
	}

	var validatedChildren []models.TopicNodeAI
	for _, c := range childrenRaw {
		vc, err := validateNode(c, depth+1)
		if err != nil {
			return nil, err
		}
		if vc != nil {
			validatedChildren = append(validatedChildren, *vc)
		}
	}

	seenNames := make(map[string]bool)
	deduped := make([]models.TopicNodeAI, 0, len(validatedChildren))
	for _, child := range validatedChildren {
		lower := strings.ToLower(strings.TrimSpace(child.Name))
		if !seenNames[lower] {
			seenNames[lower] = true
			deduped = append(deduped, child)
		}
	}

	return &models.TopicNodeAI{
		Name:     name,
		Children: deduped,
	}, nil
}

func countNodesAI(topics []models.TopicNodeAI) int {
	count := 0
	stack := make([]models.TopicNodeAI, len(topics))
	copy(stack, topics)
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		count++
		if count > MaxTreeNodes {
			return count
		}
		stack = append(stack, node.Children...)
	}
	return count
}

func ValidateTree(data map[string]any) (*models.GenerateSyllabusOut, error) {
	if data == nil {
		return nil, core.NewAppError(502, "ai_invalid_response", "AI returned invalid response data.")
	}
	skillName, _ := data["skill_name"].(string)
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		skillName = "Untitled Skill"
	}
	if runes := []rune(skillName); len(runes) > 200 {
		skillName = string(runes[:200])
	}

	topicsRaw, ok := data["topics"].([]any)
	if !ok || len(topicsRaw) == 0 {
		return nil, core.NewAppError(502, "ai_invalid_response", "AI returned an empty or invalid syllabus.")
	}

	var validatedTopics []models.TopicNodeAI
	for _, t := range topicsRaw {
		vt, err := validateNode(t, 0)
		if err != nil {
			return nil, err
		}
		if vt != nil {
			validatedTopics = append(validatedTopics, *vt)
		}
	}

	if len(validatedTopics) == 0 {
		return nil, core.NewAppError(502, "ai_invalid_response", "AI returned an empty syllabus.")
	}

	total := countNodesAI(validatedTopics)
	if total > MaxTreeNodes {
		return nil, core.NewAppError(
			502,
			"ai_invalid_response",
			fmt.Sprintf("AI generated too many topics (%d). Please select a lower syllabus depth.", total),
		)
	}

	return &models.GenerateSyllabusOut{
		SkillName: skillName,
		Topics:    validatedTopics,
	}, nil
}

func HandleGeminiError(err error) error {
	var appErr *core.AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	excStr := strings.ToLower(err.Error())
	core.Logger.Error("Gemini API error", "err", err.Error())

	if strings.Contains(excStr, "api_key") ||
		strings.Contains(excStr, "api key") ||
		strings.Contains(excStr, "unauthenticated") ||
		strings.Contains(excStr, "permission_denied") ||
		strings.Contains(excStr, "permission denied") {
		return core.NewAppError(503, "ai_config_error", "AI service configuration error. Please contact the administrator.")
	}
	if strings.Contains(excStr, "quota") ||
		strings.Contains(excStr, "resource_exhausted") ||
		strings.Contains(excStr, "rate limit") ||
		strings.Contains(excStr, "rate_limit") ||
		strings.Contains(excStr, "too many requests") ||
		strings.Contains(excStr, "429") {
		return core.NewAppError(429, "ai_rate_limited", "AI generation rate limit or quota exceeded. Please wait a moment and try again.")
	}
	if strings.Contains(excStr, "not found") ||
		strings.Contains(excStr, "404") ||
		strings.Contains(excStr, "no longer available") ||
		strings.Contains(excStr, "is not found") {
		return core.NewAppError(503, "ai_model_unavailable", "The requested AI model is unavailable. Please set GEMINI_MODEL to gemini-3.8-flash.")
	}
	if strings.Contains(excStr, "high demand") ||
		strings.Contains(excStr, "overloaded") ||
		strings.Contains(excStr, "503") ||
		strings.Contains(excStr, "unavailable") {
		return core.NewAppError(503, "ai_overloaded", "The AI model is temporarily experiencing high demand. Please try again in a few seconds.")
	}
	if strings.Contains(excStr, "timeout") ||
		strings.Contains(excStr, "deadline") ||
		errors.Is(err, context.DeadlineExceeded) {
		return core.NewAppError(504, "ai_timeout", "AI generation timed out. Please try again.")
	}
	if strings.Contains(excStr, "safety") || strings.Contains(excStr, "blocked") {
		return core.NewAppError(422, "ai_blocked", "The syllabus request was blocked by content safety filters. Please try a different subject.")
	}
	return core.NewAppError(502, "ai_error", "AI generation failed. Please try again.")
}

func BuildCandidateModels(configuredModel string) []string {
	configured := strings.TrimSpace(configuredModel)
	configured = strings.TrimPrefix(configured, "models/")
	if mapped, ok := retiredModelMap[configured]; ok {
		configured = mapped
	}
	var candidates []string
	if configured != "" {
		candidates = append(candidates, configured)
	}
	for _, fb := range []string{"gemini-3.8-flash", "gemini-3.5-flash-lite", "gemini-3.7-flash"} {
		exists := false
		for _, c := range candidates {
			if c == fb {
				exists = true
				break
			}
		}
		if !exists {
			candidates = append(candidates, fb)
		}
	}
	return candidates
}

func thinkingConfigsForModel(modelName string) []map[string]any {
	lower := strings.ToLower(modelName)
	if strings.Contains(lower, "lite") {
		return []map[string]any{nil}
	}
	if strings.Contains(lower, "gemini-3") || strings.Contains(lower, "gemini-4") {
		return []map[string]any{{"thinkingLevel": "LOW"}, nil}
	}
	if strings.Contains(lower, "2.5") {
		return []map[string]any{{"thinkingBudget": 0}, nil}
	}
	return []map[string]any{nil}
}

var GeminiHTTPDoFunc = func(req *http.Request) (*http.Response, error) {
	client := &http.Client{Timeout: PerAttemptTimeout}
	return client.Do(req)
}

var CallGeminiFunc = defaultCallGemini

func discoverAvailableFlashModels(ctx context.Context, apiKey string) []string {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, GeminiRestBase, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("x-goog-api-key", apiKey)

	res, err := GeminiHTTPDoFunc(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil
	}
	var data map[string]any
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return nil
	}
	modelsArr, _ := data["models"].([]any)
	var flashModels []string
	var otherModels []string
	for _, item := range modelsArr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		methods, _ := m["supportedGenerationMethods"].([]any)
		supports := false
		for _, meth := range methods {
			if s, ok := meth.(string); ok && s == "generateContent" {
				supports = true
				break
			}
		}
		if !supports {
			continue
		}
		name, _ := m["name"].(string)
		name = strings.TrimPrefix(name, "models/")
		if name == "" || strings.Contains(name, "tts") || strings.Contains(name, "image") || strings.Contains(name, "embedding") || strings.Contains(name, "vision") {
			continue
		}
		if strings.Contains(name, "flash") {
			flashModels = append(flashModels, name)
		} else if strings.Contains(name, "gemini") {
			otherModels = append(otherModels, name)
		}
	}
	return append(flashModels, otherModels...)
}

func defaultCallGemini(ctx context.Context, cfg *config.Settings, prompt string, temperature float64) (string, error) {
	apiKey := strings.TrimSpace(cfg.GeminiAPIKey)
	if apiKey == "" {
		return "", core.NewAppError(
			503,
			"ai_unavailable",
			"AI generation is not configured on this server (GEMINI_API_KEY missing).",
		)
	}

	deadline := time.Now().Add(TotalGenerationBudget)
	candidates := BuildCandidateModels(cfg.GeminiModel)
	tried := make(map[string]bool)
	var lastExc error
	var non404Exc error
	discovered := false

	idx := 0
	for idx < len(candidates) {
		modelName := candidates[idx]
		idx++
		if tried[modelName] {
			continue
		}
		tried[modelName] = true

		remaining := time.Until(deadline)
		if remaining <= 3*time.Second {
			break
		}

		url := fmt.Sprintf("%s/%s:generateContent", GeminiRestBase, modelName)
		thinkingOptions := thinkingConfigsForModel(modelName)

		for _, thinkingCfg := range thinkingOptions {
			genConfig := map[string]any{
				"responseMimeType": "application/json",
				"maxOutputTokens":  8192,
			}
			if !strings.Contains(strings.ToLower(modelName), "lite") {
				genConfig["temperature"] = temperature
			}
			if thinkingCfg != nil {
				genConfig["thinkingConfig"] = thinkingCfg
			}

			payload := map[string]any{
				"systemInstruction": map[string]any{
					"parts": []map[string]any{{"text": SystemPrompt}},
				},
				"contents": []map[string]any{
					{
						"role":  "user",
						"parts": []map[string]any{{"text": prompt}},
					},
				},
				"generationConfig": genConfig,
			}

			bodyBytes, _ := json.Marshal(payload)
			reqCtx, cancel := context.WithTimeout(ctx, PerAttemptTimeout)
			req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(bodyBytes))
			if err != nil {
				cancel()
				lastExc = err
				non404Exc = err
				break
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("x-goog-api-key", apiKey)

			res, err := GeminiHTTPDoFunc(req)
			if err != nil {
				cancel()
				lastExc = err
				non404Exc = err
				break
			}
			respBytes, readErr := io.ReadAll(res.Body)
			res.Body.Close()
			cancel()
			if readErr != nil {
				lastExc = readErr
				non404Exc = readErr
				break
			}

			if res.StatusCode == http.StatusOK {
				var body map[string]any
				_ = json.Unmarshal(respBytes, &body)
				respCandidates, _ := body["candidates"].([]any)
				if len(respCandidates) == 0 {
					if pf, ok := body["promptFeedback"].(map[string]any); ok {
						if br, ok := pf["blockReason"].(string); ok && br != "" {
							return "", core.NewAppError(
								422,
								"ai_blocked",
								"The syllabus request was blocked by content safety filters. Please try a different subject.",
							)
						}
					}
					return "", core.NewAppError(502, "ai_empty_response", "AI returned an empty response. Please try again.")
				}

				first, _ := respCandidates[0].(map[string]any)
				finishReason, _ := first["finishReason"].(string)
				switch strings.ToUpper(finishReason) {
				case "SAFETY", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
					return "", core.NewAppError(
						422,
						"ai_blocked",
						"The syllabus request was blocked by content safety filters. Please try a different subject.",
					)
				}

				content, _ := first["content"].(map[string]any)
				parts, _ := content["parts"].([]any)
				var textChunks []string
				for _, p := range parts {
					if pm, ok := p.(map[string]any); ok {
						if thought, ok := pm["thought"].(bool); ok && thought {
							continue
						}
						if t, ok := pm["text"].(string); ok && t != "" {
							textChunks = append(textChunks, t)
						}
					}
				}
				text := strings.TrimSpace(strings.Join(textChunks, ""))
				if text == "" {
					return "", core.NewAppError(502, "ai_empty_response", "AI returned an empty response. Please try again.")
				}
				return text, nil
			}

			var errEnvelope map[string]any
			_ = json.Unmarshal(respBytes, &errEnvelope)
			errData, _ := errEnvelope["error"].(map[string]any)
			errMsg, _ := errData["message"].(string)
			if errMsg == "" {
				errMsg = string(respBytes)
			}
			errStatus, _ := errData["status"].(string)

			if res.StatusCode == 400 && thinkingCfg != nil {
				continue
			}

			lastExc = fmt.Errorf("%d %s: %s", res.StatusCode, errStatus, errMsg)
			if res.StatusCode != 404 {
				non404Exc = lastExc
			}
			errLower := strings.ToLower(fmt.Sprintf("%d %s %s", res.StatusCode, errStatus, errMsg))

			if res.StatusCode == 401 || res.StatusCode == 403 ||
				strings.Contains(errLower, "api_key_invalid") ||
				strings.Contains(errLower, "api key not valid") {
				return "", HandleGeminiError(lastExc)
			}

			if res.StatusCode == 404 {
				matches := reModelSuggest.FindAllStringSubmatch(errMsg, -1)
				for _, m := range matches {
					if len(m) > 1 {
						suggested := m[1]
						if suggested != modelName && !tried[suggested] {
							exists := false
							for _, c := range candidates {
								if c == suggested {
									exists = true
									break
								}
							}
							if !exists {
								// Insert at current idx
								candidates = append(candidates[:idx], append([]string{suggested}, candidates[idx:]...)...)
							}
						}
					}
				}
			}

			if idx >= len(candidates) && non404Exc == nil && !discovered {
				discovered = true
				for _, discModel := range discoverAvailableFlashModels(ctx, apiKey) {
					if !tried[discModel] {
						exists := false
						for _, c := range candidates {
							if c == discModel {
								exists = true
								break
							}
						}
						if !exists {
							candidates = append(candidates, discModel)
						}
					}
				}
			}

			isFallbackEligible := res.StatusCode == 404 ||
				res.StatusCode == 429 ||
				res.StatusCode == 500 ||
				res.StatusCode == 502 ||
				res.StatusCode == 503 ||
				res.StatusCode == 504 ||
				strings.Contains(errLower, "not found") ||
				strings.Contains(errLower, "not supported") ||
				strings.Contains(errLower, "no longer available")

			if isFallbackEligible && idx < len(candidates) {
				break
			}

			if non404Exc != nil {
				return "", HandleGeminiError(non404Exc)
			}
			return "", HandleGeminiError(lastExc)
		}
	}

	if non404Exc != nil {
		return "", HandleGeminiError(non404Exc)
	}
	if lastExc != nil {
		return "", HandleGeminiError(lastExc)
	}
	return "", core.NewAppError(504, "ai_timeout", "AI generation timed out. Please try again.")
}

func GenerateSyllabus(ctx context.Context, cfg *config.Settings, subject, depth, purpose string) (*models.GenerateSyllabusOut, error) {
	start := time.Now()
	prompt := BuildGenerationPrompt(subject, depth, purpose)
	raw, err := CallGeminiFunc(ctx, cfg, prompt, 0.5)
	if err != nil {
		return nil, HandleGeminiError(err)
	}

	data, err := ExtractJSON(raw)
	if err != nil {
		if TotalGenerationBudget-time.Since(start) < 12*time.Second {
			return nil, err
		}
		retryPrompt := "Your previous response was not valid JSON. Return ONLY the valid compact JSON object with 'skill_name' and 'topics'.\n\n" + prompt
		raw2, err2 := CallGeminiFunc(ctx, cfg, retryPrompt, 0.2)
		if err2 != nil {
			return nil, HandleGeminiError(err2)
		}
		data, err = ExtractJSON(raw2)
		if err != nil {
			return nil, err
		}
	}

	return ValidateTree(data)
}

func RefineSyllabus(
	ctx context.Context,
	cfg *config.Settings,
	subject string,
	depth string,
	purpose string,
	currentTree map[string]any,
	conversationHistory []map[string]string,
	instruction string,
) (*models.GenerateSyllabusOut, error) {
	start := time.Now()
	recent := conversationHistory
	if len(recent) > 15 {
		recent = recent[len(recent)-15:]
	}
	var historyLines []string
	for _, msg := range recent {
		role := msg["role"]
		if role == "" {
			role = "user"
		}
		content := msg["content"]
		if role == "user" {
			historyLines = append(historyLines, fmt.Sprintf("User requested: %s", content))
		} else {
			historyLines = append(historyLines, "Assistant updated the syllabus accordingly.")
		}
	}

	prompt := BuildRefinementPrompt(subject, depth, purpose, currentTree, instruction)
	if len(historyLines) > 0 {
		prompt = "Conversation History:\n" + strings.Join(historyLines, "\n") + "\n\n" + prompt
	}

	raw, err := CallGeminiFunc(ctx, cfg, prompt, 0.3)
	if err != nil {
		return nil, HandleGeminiError(err)
	}

	data, err := ExtractJSON(raw)
	if err != nil {
		if TotalGenerationBudget-time.Since(start) < 12*time.Second {
			return nil, err
		}
		retryPrompt := "Your previous response was not valid JSON. Return ONLY the valid compact JSON object with 'skill_name' and 'topics'.\n\n" + prompt
		raw2, err2 := CallGeminiFunc(ctx, cfg, retryPrompt, 0.1)
		if err2 != nil {
			return nil, HandleGeminiError(err2)
		}
		data, err = ExtractJSON(raw2)
		if err != nil {
			return nil, err
		}
	}

	return ValidateTree(data)
}
