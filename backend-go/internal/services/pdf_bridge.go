package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"skill-progress-tracker/backend-go/internal/config"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
)

type bridgeOutput struct {
	OK           bool           `json:"ok"`
	Tree         map[string]any `json:"tree"`
	ErrorCode    string         `json:"error_code"`
	ErrorMessage string         `json:"error_message"`
}

func ComputeBridgeHMAC(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// SanitizePDFObjectFilename validates that the filename has a .pdf extension and returns
// a clean, path-traversal-safe basename suitable for private object storage keys.
func SanitizePDFObjectFilename(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", core.NewAppError(http.StatusBadRequest, "invalid_pdf", "Filename is required.")
	}
	if strings.Contains(trimmed, "..") || strings.ContainsAny(trimmed, `/\`) {
		return "", core.NewAppError(http.StatusBadRequest, "invalid_pdf", "Invalid PDF filename.")
	}
	base := filepath.Base(trimmed)
	if !strings.HasSuffix(strings.ToLower(base), ".pdf") {
		return "", core.NewAppError(http.StatusBadRequest, "invalid_pdf", "Only .pdf files are supported.")
	}
	var sb strings.Builder
	for _, r := range base {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			sb.WriteRune(r)
		} else {
			sb.WriteByte('_')
		}
	}
	cleaned := strings.TrimLeft(sb.String(), ".")
	if cleaned == "" || strings.ToLower(cleaned) == "pdf" {
		cleaned = "syllabus.pdf"
	}
	if !strings.HasSuffix(strings.ToLower(cleaned), ".pdf") {
		cleaned += ".pdf"
	}
	if len(cleaned) > 120 {
		cleaned = cleaned[len(cleaned)-120:]
		cleaned = strings.TrimLeft(cleaned, ".")
	}
	return cleaned, nil
}

// ValidateOwnedObjectKey enforces strict user isolation, path traversal prevention,
// and SSRF rejection on storage object keys. Every key must have the exact form:
// "<user_uuid>/<upload_uuid>/<safe_filename.pdf>" owned by userID.
func ValidateOwnedObjectKey(objectKey string, userID uuid.UUID) error {
	key := strings.TrimSpace(objectKey)
	if key == "" {
		return core.NewAppError(http.StatusForbidden, "forbidden", "Invalid storage object key.")
	}
	lower := strings.ToLower(key)
	if strings.Contains(lower, "://") ||
		strings.HasPrefix(lower, "http:") ||
		strings.HasPrefix(lower, "https:") ||
		strings.Contains(key, "..") ||
		strings.Contains(key, `\`) ||
		strings.HasPrefix(key, "/") {
		return core.NewAppError(http.StatusForbidden, "forbidden", "Invalid storage object key.")
	}
	parts := strings.Split(key, "/")
	if len(parts) != 3 {
		return core.NewAppError(http.StatusForbidden, "forbidden", "Invalid storage object key structure.")
	}
	if parts[0] != userID.String() {
		return core.NewAppError(http.StatusForbidden, "forbidden", "You do not have permission to access this storage object.")
	}
	if _, err := uuid.Parse(parts[1]); err != nil {
		return core.NewAppError(http.StatusForbidden, "forbidden", "Invalid storage upload identifier.")
	}
	safeName, err := SanitizePDFObjectFilename(parts[2])
	if err != nil || safeName != parts[2] {
		return core.NewAppError(http.StatusForbidden, "forbidden", "Invalid storage object filename.")
	}
	return nil
}

func SignUploadToken(secret string, userID uuid.UUID, objectKey string, fileSizeBytes int64, expiresAt int64) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", errors.New("INTERNAL_PARSER_SECRET is not configured")
	}
	payload := fmt.Sprintf("%s|%s|%d|%d", userID.String(), objectKey, fileSizeBytes, expiresAt)
	sig := ComputeBridgeHMAC(secret, []byte(payload))
	return fmt.Sprintf("%d.%s", expiresAt, sig), nil
}

func VerifyUploadToken(secret string, token string, userID uuid.UUID, objectKey string, fileSizeBytes int64, nowUnix int64) error {
	if strings.TrimSpace(secret) == "" {
		return core.NewAppError(http.StatusServiceUnavailable, "parser_unconfigured", "Internal PDF parser secret is not configured.")
	}
	parts := strings.SplitN(strings.TrimSpace(token), ".", 2)
	if len(parts) != 2 {
		return core.NewAppError(http.StatusForbidden, "forbidden", "Invalid upload token.")
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return core.NewAppError(http.StatusForbidden, "forbidden", "Invalid upload token expiration.")
	}
	if nowUnix > exp {
		return core.NewAppError(http.StatusForbidden, "upload_expired", "Upload session has expired. Please upload the PDF again.")
	}
	payload := fmt.Sprintf("%s|%s|%d|%d", userID.String(), objectKey, fileSizeBytes, exp)
	expectedSig := ComputeBridgeHMAC(secret, []byte(payload))
	if !hmac.Equal([]byte(parts[1]), []byte(expectedSig)) {
		return core.NewAppError(http.StatusForbidden, "forbidden", "Invalid upload token signature.")
	}
	return nil
}

func escapeObjectKeyPath(objectKey string) string {
	parts := strings.Split(objectKey, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func CreateSupabaseSignedUploadURL(ctx context.Context, cfg *config.Settings, objectKey string) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.SupabaseURL), "/")
	bucket := url.PathEscape(strings.TrimSpace(cfg.SupabaseStorageBucket))
	endpoint := fmt.Sprintf("%s/storage/v1/object/upload/sign/%s/%s", baseURL, bucket, escapeObjectKeyPath(objectKey))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte(`{}`)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.SupabaseServiceRoleKey))
	req.Header.Set("apikey", strings.TrimSpace(cfg.SupabaseServiceRoleKey))

	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	bodyBytes, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("supabase storage sign upload returned status %d", res.StatusCode)
	}
	var parsed map[string]any
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return "", err
	}
	relURL, _ := parsed["url"].(string)
	if relURL == "" {
		relURL, _ = parsed["signedURL"].(string)
	}
	if relURL == "" {
		return "", errors.New("supabase storage response missing signed url")
	}
	if strings.HasPrefix(relURL, "http://") || strings.HasPrefix(relURL, "https://") {
		return relURL, nil
	}
	if !strings.HasPrefix(relURL, "/storage/v1") {
		relURL = "/storage/v1" + relURL
	}
	return baseURL + relURL, nil
}

func CreateSupabaseSignedDownloadURL(ctx context.Context, cfg *config.Settings, objectKey string, expiresInSec int) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.SupabaseURL), "/")
	bucket := url.PathEscape(strings.TrimSpace(cfg.SupabaseStorageBucket))
	endpoint := fmt.Sprintf("%s/storage/v1/object/sign/%s/%s", baseURL, bucket, escapeObjectKeyPath(objectKey))

	payload, _ := json.Marshal(map[string]any{"expiresIn": expiresInSec})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.SupabaseServiceRoleKey))
	req.Header.Set("apikey", strings.TrimSpace(cfg.SupabaseServiceRoleKey))

	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	bodyBytes, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("supabase storage sign download returned status %d", res.StatusCode)
	}
	var parsed map[string]any
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return "", err
	}
	relURL, _ := parsed["signedURL"].(string)
	if relURL == "" {
		relURL, _ = parsed["url"].(string)
	}
	if relURL == "" {
		return "", errors.New("supabase storage response missing signed download url")
	}
	if strings.HasPrefix(relURL, "http://") || strings.HasPrefix(relURL, "https://") {
		return relURL, nil
	}
	if !strings.HasPrefix(relURL, "/storage/v1") {
		relURL = "/storage/v1" + relURL
	}
	return baseURL + relURL, nil
}

func DeleteSupabaseStorageObject(ctx context.Context, cfg *config.Settings, objectKey string) error {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.SupabaseURL), "/")
	bucket := url.PathEscape(strings.TrimSpace(cfg.SupabaseStorageBucket))
	endpoint := fmt.Sprintf("%s/storage/v1/object/%s/%s", baseURL, bucket, escapeObjectKeyPath(objectKey))

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.SupabaseServiceRoleKey))
	req.Header.Set("apikey", strings.TrimSpace(cfg.SupabaseServiceRoleKey))

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return nil
}

var InvokePythonPDFParserFunc = defaultInvokePythonPDFParser

func InvokePythonPDFParser(ctx context.Context, cfg *config.Settings, rawPDF []byte) (map[string]any, string) {
	return InvokePythonPDFParserFunc(ctx, cfg, rawPDF)
}

func defaultInvokePythonPDFParser(ctx context.Context, cfg *config.Settings, rawPDF []byte) (map[string]any, string) {
	secret := cfg.EffectiveInternalSecret()
	if secret == "" {
		core.Logger.Error("PDF parser invocation blocked: INTERNAL_PARSER_SECRET is not configured")
		return nil, "Internal PDF parser authentication is not configured."
	}
	if parserURL := cfg.EffectivePythonParserURL(); parserURL != "" {
		return invokeRemotePythonBridge(ctx, cfg, parserURL, rawPDF)
	}
	if cfg.IsServerless() {
		core.Logger.Error("PDF parser invocation blocked: serverless runtime requires PYTHON_PARSER_URL or VERCEL_URL")
		return nil, "Internal PDF parser URL is not configured for serverless runtime."
	}
	return invokeSubprocessPythonBridge(ctx, cfg, rawPDF)
}

// InvokePythonPDFParserFromStorageURL instructs the remote Python serverless endpoint
// to fetch the PDF directly from a short-lived signed private storage URL, keeping
// 10 MB PDFs completely out of Vercel function request bodies.
func InvokePythonPDFParserFromStorageURL(ctx context.Context, cfg *config.Settings, downloadURL, objectKey string) (map[string]any, string, string) {
	secret := cfg.EffectiveInternalSecret()
	if secret == "" {
		core.Logger.Error("PDF parser storage invocation blocked: INTERNAL_PARSER_SECRET is not configured")
		return nil, "parser_unconfigured", "Internal PDF parser authentication is not configured."
	}
	parserURL := cfg.EffectivePythonParserURL()
	if parserURL == "" {
		return nil, "parser_unconfigured", "Internal PDF parser URL is not configured."
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	payloadBytes, err := json.Marshal(map[string]any{
		"mode":         "storage_url",
		"download_url": downloadURL,
		"object_key":   objectKey,
		"max_bytes":    cfg.MaxPDFBytes,
	})
	if err != nil {
		return nil, "internal_error", "Parsing failed. The file was not stored as a skill."
	}

	ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	macInput := append([]byte(ts+"."), payloadBytes...)
	sig := ComputeBridgeHMAC(secret, macInput)

	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodPost, parserURL, bytes.NewReader(payloadBytes))
	if err != nil {
		core.Logger.Error("Remote PDF parser storage request creation failed", "err", err, "url", parserURL)
		return nil, "internal_error", "Parsing failed. The file was not stored as a skill."
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Bridge-Timestamp", ts)
	req.Header.Set("X-Internal-Bridge-Signature", sig)
	if bypass := strings.TrimSpace(os.Getenv("VERCEL_AUTOMATION_BYPASS_SECRET")); bypass != "" {
		req.Header.Set("x-vercel-protection-bypass", bypass)
	} else if bypass := strings.TrimSpace(os.Getenv("VERCEL_PROTECTION_BYPASS")); bypass != "" {
		req.Header.Set("x-vercel-protection-bypass", bypass)
	}

	client := &http.Client{
		Timeout: 45 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	res, err := client.Do(req)
	if err != nil {
		core.Logger.Error("Remote PDF parser storage request failed", "err", err, "url", parserURL)
		return nil, "internal_error", "Parsing failed. The file was not stored as a skill."
	}
	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		core.Logger.Error("Remote PDF parser storage read body failed", "err", err, "status", res.StatusCode)
		return nil, "internal_error", "Parsing failed. The file was not stored as a skill."
	}

	if res.StatusCode != http.StatusOK {
		core.Logger.Error("Remote PDF parser storage non-200 response", "status", res.StatusCode, "url", parserURL, "body", string(bodyBytes))
		var errResp bridgeOutput
		if json.Unmarshal(bodyBytes, &errResp) == nil && strings.TrimSpace(errResp.ErrorMessage) != "" {
			code := errResp.ErrorCode
			if code == "" {
				code = "internal_error"
			}
			return nil, code, errResp.ErrorMessage
		}
		if res.StatusCode == http.StatusFound || res.StatusCode == http.StatusMovedPermanently || res.StatusCode == http.StatusTemporaryRedirect {
			return nil, "internal_error", "PDF parser endpoint was redirected. Please check Vercel deployment protection or PYTHON_PARSER_URL."
		}
		return nil, "internal_error", fmt.Sprintf("PDF parser service error (HTTP %d).", res.StatusCode)
	}

	var resp bridgeOutput
	if err := json.Unmarshal(bodyBytes, &resp); err != nil {
		core.Logger.Error("Remote PDF parser storage invalid JSON response", "err", err, "status", res.StatusCode, "body", string(bodyBytes))
		return nil, "internal_error", "Parsing failed. The file was not stored as a skill."
	}
	if !resp.OK {
		msg := resp.ErrorMessage
		if msg == "" {
			msg = "Parsing failed. The file was not stored as a skill."
		}
		return nil, resp.ErrorCode, msg
	}
	return resp.Tree, "", ""
}

func invokeSubprocessPythonBridge(ctx context.Context, cfg *config.Settings, rawPDF []byte) (map[string]any, string) {
	secret := cfg.EffectiveInternalSecret()
	if secret == "" {
		return nil, "Internal PDF parser authentication is not configured."
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	signature := ComputeBridgeHMAC(secret, rawPDF)

	cmd := exec.CommandContext(timeoutCtx, cfg.PythonBin, cfg.PythonBridgeScript)
	cmd.Stdin = bytes.NewReader(rawPDF)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	env := os.Environ()
	env = append(env,
		"INTERNAL_PARSER_SECRET="+secret,
		"BRIDGE_HMAC_SECRET="+secret,
		"BRIDGE_HMAC_SIGNATURE="+signature,
		"BRIDGE_MAX_PDF_PAGES="+strconv.Itoa(cfg.MaxPDFPages),
		"ANTHROPIC_API_KEY="+cfg.AnthropicAPIKey,
	)
	cmd.Env = env

	_ = cmd.Run()
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return nil, "Parsing failed. The file was not stored as a skill."
	}

	var resp bridgeOutput
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		core.Logger.Error("PDF bridge failed", "stderr", stderr.String())
		return nil, "Parsing failed. The file was not stored as a skill."
	}
	if !resp.OK {
		msg := resp.ErrorMessage
		if msg == "" {
			msg = "Parsing failed. The file was not stored as a skill."
		}
		return nil, msg
	}
	return resp.Tree, ""
}

func invokeRemotePythonBridge(ctx context.Context, cfg *config.Settings, parserURL string, rawPDF []byte) (map[string]any, string) {
	secret := cfg.EffectiveInternalSecret()
	if secret == "" {
		return nil, "Internal PDF parser authentication is not configured."
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	macInput := append([]byte(ts+"."), rawPDF...)
	sig := ComputeBridgeHMAC(secret, macInput)

	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodPost, parserURL, bytes.NewReader(rawPDF))
	if err != nil {
		core.Logger.Error("Remote PDF parser request construction failed", "err", err, "url", parserURL)
		return nil, "Parsing failed. The file was not stored as a skill."
	}
	req.Header.Set("Content-Type", "application/pdf")
	req.Header.Set("X-Internal-Bridge-Timestamp", ts)
	req.Header.Set("X-Internal-Bridge-Signature", sig)
	if bypass := strings.TrimSpace(os.Getenv("VERCEL_AUTOMATION_BYPASS_SECRET")); bypass != "" {
		req.Header.Set("x-vercel-protection-bypass", bypass)
	} else if bypass := strings.TrimSpace(os.Getenv("VERCEL_PROTECTION_BYPASS")); bypass != "" {
		req.Header.Set("x-vercel-protection-bypass", bypass)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	res, err := client.Do(req)
	if err != nil {
		core.Logger.Error("Remote PDF parser request failed", "err", err, "url", parserURL)
		return nil, "Parsing failed. The file was not stored as a skill."
	}
	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		core.Logger.Error("Remote PDF parser failed to read response body", "err", err, "status", res.StatusCode)
		return nil, "Parsing failed. The file was not stored as a skill."
	}

	if res.StatusCode != http.StatusOK {
		core.Logger.Error("Remote PDF parser non-200 response", "status", res.StatusCode, "url", parserURL, "body", string(bodyBytes))
		var errResp bridgeOutput
		if json.Unmarshal(bodyBytes, &errResp) == nil && strings.TrimSpace(errResp.ErrorMessage) != "" {
			return nil, errResp.ErrorMessage
		}
		if res.StatusCode == http.StatusFound || res.StatusCode == http.StatusMovedPermanently || res.StatusCode == http.StatusTemporaryRedirect {
			return nil, "PDF parser endpoint was redirected. Please check Vercel deployment protection or PYTHON_PARSER_URL."
		}
		return nil, fmt.Sprintf("PDF parser service error (HTTP %d).", res.StatusCode)
	}

	var resp bridgeOutput
	if err := json.Unmarshal(bodyBytes, &resp); err != nil {
		core.Logger.Error("Remote PDF parser invalid JSON response", "err", err, "status", res.StatusCode, "body", string(bodyBytes))
		return nil, "Parsing failed. The file was not stored as a skill."
	}
	if !resp.OK {
		msg := resp.ErrorMessage
		if msg == "" {
			msg = "Parsing failed. The file was not stored as a skill."
		}
		return nil, msg
	}
	return resp.Tree, ""
}

func toMapSlice(raw any) []map[string]any {
	if s, ok := raw.([]map[string]any); ok {
		return s
	}
	if arr, ok := raw.([]any); ok {
		var out []map[string]any
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

func SanitizePDFNodes(nodesRaw any, depth int) ([]map[string]any, error) {
	if depth > MaxTreeDepth {
		return nil, core.NewAppError(400, "invalid_tree", "Import tree exceeds the maximum depth.")
	}
	nodes := toMapSlice(nodesRaw)
	out := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		name := "Untitled"
		if rawName, ok := node["name"].(string); ok && rawName != "" {
			name = rawName
		} else if rawNameAny := node["name"]; rawNameAny != nil {
			s := fmt.Sprintf("%v", rawNameAny)
			if s != "" {
				name = s
			}
		}
		if runes := []rune(name); len(runes) > MaxNameLen {
			name = string(runes[:MaxNameLen])
		}

		var desc any
		if node["description"] != nil {
			dStr := fmt.Sprintf("%v", node["description"])
			if runes := []rune(dStr); len(runes) > 2000 {
				dStr = string(runes[:2000])
			}
			desc = dStr
		}

		children, err := SanitizePDFNodes(node["children"], depth+1)
		if err != nil {
			return nil, err
		}

		out = append(out, map[string]any{
			"name":        name,
			"description": desc,
			"confidence":  node["confidence"],
			"source":      node["source"],
			"children":    children,
		})
	}
	return out, nil
}

func SanitizePDFTree(tree map[string]any) (map[string]any, error) {
	if tree == nil {
		return nil, core.NewAppError(400, "invalid_tree", "Import tree is invalid.")
	}
	name := "Imported skill"
	if rawName, ok := tree["name"].(string); ok && rawName != "" {
		name = rawName
	}
	if runes := []rune(name); len(runes) > MaxNameLen {
		name = string(runes[:MaxNameLen])
	}
	children, err := SanitizePDFNodes(tree["children"], 0)
	if err != nil {
		return nil, err
	}
	lowConf := tree["low_confidence_sections"]
	if lowConf == nil {
		lowConf = []any{}
	}
	return map[string]any{
		"name":                    name,
		"children":                children,
		"confidence_notes":        tree["confidence_notes"],
		"low_confidence_sections": lowConf,
	}, nil
}

func CountMapNodes(nodesRaw any) int {
	total := 0
	stack := toMapSlice(nodesRaw)
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		total++
		if total > MaxTreeNodes {
			return total
		}
		stack = append(stack, toMapSlice(node["children"])...)
	}
	return total
}

func AssertPDFTreeBudget(tree map[string]any) error {
	count := CountMapNodes(tree["children"])
	if count > MaxTreeNodes {
		return core.NewAppError(400, "invalid_tree", "Import tree has too many topics.")
	}
	return nil
}

type topicToInsert struct {
	id          uuid.UUID
	skillID     uuid.UUID
	parentID    *uuid.UUID
	name        string
	description *string
	orderIndex  int
}

type queueItem struct {
	nodes    []map[string]any
	parentID *uuid.UUID
}

func PersistTopicNodes(
	ctx context.Context,
	db *database.DB,
	q database.Querier,
	skillID uuid.UUID,
	nodes []map[string]any,
	parentID *uuid.UUID,
) (int, error) {
	if len(nodes) == 0 {
		return 0, nil
	}

	// Group topics level-by-level so every parent is guaranteed to be inserted
	// before any of its children, completely satisfying foreign key constraints.
	var levels [][]topicToInsert
	currentLevelQueue := []queueItem{{nodes: nodes, parentID: parentID}}

	for len(currentLevelQueue) > 0 {
		var currentLevelTopics []topicToInsert
		var nextLevelQueue []queueItem

		for _, item := range currentLevelQueue {
			for idx, node := range item.nodes {
				name, _ := node["name"].(string)
				name = strings.TrimSpace(name)
				if name == "" {
					name = "Untitled"
				}
				if runes := []rune(name); len(runes) > MaxNameLen {
					name = string(runes[:MaxNameLen])
				}

				var descPtr *string
				if dStr, ok := node["description"].(string); ok && strings.TrimSpace(dStr) != "" {
					d := strings.TrimSpace(dStr)
					if runes := []rune(d); len(runes) > 2000 {
						d = string(runes[:2000])
					}
					descPtr = &d
				}

				topicID := uuid.New()
				currentLevelTopics = append(currentLevelTopics, topicToInsert{
					id:          topicID,
					skillID:     skillID,
					parentID:    item.parentID,
					name:        name,
					description: descPtr,
					orderIndex:  idx,
				})

				children := toMapSlice(node["children"])
				if len(children) > 0 {
					pid := topicID
					nextLevelQueue = append(nextLevelQueue, queueItem{
						nodes:    children,
						parentID: &pid,
					})
				}
			}
		}

		if len(currentLevelTopics) > 0 {
			levels = append(levels, currentLevelTopics)
		}
		currentLevelQueue = nextLevelQueue
	}

	now := time.Now().UTC()
	nowVal := db.FormatTime(now)
	created := 0
	const batchSize = 50

	for _, level := range levels {
		for i := 0; i < len(level); i += batchSize {
			end := i + batchSize
			if end > len(level) {
				end = len(level)
			}
			batch := level[i:end]

			var query strings.Builder
			query.WriteString(`
				INSERT INTO topics (
					id, skill_id, parent_id, name, description,
					order_index, is_completed, completed_at, created_at, updated_at
				) VALUES `)

			args := make([]any, 0, len(batch)*8)
			for j, t := range batch {
				if j > 0 {
					query.WriteString(", ")
				}
				query.WriteString("(?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)")
				args = append(args,
					db.FormatUUID(t.id),
					db.FormatUUID(t.skillID),
					db.FormatNullUUID(t.parentID),
					t.name,
					t.description,
					t.orderIndex,
					false,
					nowVal,
					nowVal,
				)
			}

			if _, err := db.Exec(ctx, q, query.String(), args...); err != nil {
				return created, err
			}
			created += len(batch)
		}
	}

	return created, nil
}
