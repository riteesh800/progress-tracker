package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"skill-progress-tracker/backend-go/internal/config"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/handlers"
	"skill-progress-tracker/backend-go/internal/models"
	"skill-progress-tracker/backend-go/internal/services"
)

func TestAISyllabusGenerateRefineConfirmAndJSONRepair(t *testing.T) {
	env := NewTestEnv(t)
	token, _ := env.RegisterUser("aiuser@example.com", "Password123!", "AIUser")

	// 1. Test JSON repair on truncated Gemini output
	truncated := `{"skill_name": "System Design", "topics": [{"name": "Load Balancing", "children": [{"name": "Round Robin"}]}, {"name": "Caching", "children": [{"name": "Unclosed`
	repaired, err := services.ExtractJSON(truncated)
	if err != nil || repaired == nil {
		t.Fatalf("expected truncated JSON to be repaired, got err=%v", err)
	}

	// 2. Test retired model mapping & candidate list
	candidates := services.BuildCandidateModels("gemini-1.5-flash")
	if len(candidates) == 0 || candidates[0] != "gemini-3.8-flash" {
		t.Fatalf("expected gemini-1.5-flash to map to gemini-3.8-flash, got %v", candidates)
	}

	// 3. Mock CallGeminiFunc to test /ai/generate-syllabus and /ai/refine-syllabus
	origCall := services.CallGeminiFunc
	t.Cleanup(func() { services.CallGeminiFunc = origCall })

	services.CallGeminiFunc = func(_ context.Context, _ *config.Settings, _ string, _ float64) (string, error) {
		return `{"skill_name":"Go Concurrency","topics":[{"name":"1.1 Goroutines","children":[{"name":"• Channels"},{"name":"Select Statement"}]}]}`, nil
	}

	code, genOut, _ := env.DoJSON(http.MethodPost, "/ai/generate-syllabus", map[string]any{
		"subject": "Go Concurrency",
		"depth":   "medium",
		"purpose": "interview",
	}, AuthHeader(token))
	if code != http.StatusOK || genOut["skill_name"] != "Go Concurrency" {
		t.Fatalf("generate-syllabus failed: %d %v", code, genOut)
	}
	topics, _ := genOut["topics"].([]any)
	firstTopic, _ := topics[0].(map[string]any)
	if firstTopic["name"] != "Goroutines" {
		t.Fatalf("expected prefix '1.1 ' to be stripped, got %q", firstTopic["name"])
	}

	// 4. Refine syllabus
	code, refOut, _ := env.DoJSON(http.MethodPost, "/ai/refine-syllabus", map[string]any{
		"subject":              "Go Concurrency",
		"depth":                "medium",
		"purpose":              "interview",
		"current_tree":         genOut,
		"conversation_history": []map[string]string{{"role": "user", "content": "Add Context package"}},
		"instruction":          "Add Context cancellation",
	}, AuthHeader(token))
	if code != http.StatusOK || refOut["skill_name"] != "Go Concurrency" {
		t.Fatalf("refine-syllabus failed: %d %v", code, refOut)
	}

	// 5. Confirm syllabus persists Skill and Topics
	code, skillOut, _ := env.DoJSON(http.MethodPost, "/ai/confirm-syllabus", map[string]any{
		"skill_name":        "Go Concurrency",
		"skill_description": "AI generated syllabus",
		"topics":            genOut["topics"],
	}, AuthHeader(token))
	if code != http.StatusCreated || skillOut["total_leaves"] != float64(2) {
		t.Fatalf("confirm-syllabus failed: %d %v", code, skillOut)
	}
}

func generateValidSyllabusPDFBytesWithSize(t *testing.T, env *TestEnv, targetSize int) []byte {
	t.Helper()
	pyCode := fmt.Sprintf(`
import io, sys
from pypdf import PdfWriter
from pypdf.generic import DecodedStreamObject, DictionaryObject, NameObject

target_size = %d
writer = PdfWriter()
page = writer.add_blank_page(width=612, height=792)
font = DictionaryObject({
    NameObject("/Type"): NameObject("/Font"),
    NameObject("/Subtype"): NameObject("/Type1"),
    NameObject("/BaseFont"): NameObject("/Helvetica"),
})
font_ref = writer._add_object(font)
page[NameObject("/Resources")] = DictionaryObject({
    NameObject("/Font"): DictionaryObject({NameObject("/F1"): font_ref})
})
lines = [
    "Computer Networks Syllabus",
    "Unit 1: Physical Layer",
    "1.1 Signal Propagation",
    "1.2 Multiplexing",
    "Unit 2: Data Link Layer",
    "2.1 Framing and Error Detection",
    "2.2 MAC Protocols",
]
ops = ["BT /F1 12 Tf 72 720 Td 16 TL"]
for ln in lines:
    ops.append(f"({ln}) Tj T*")
ops.append("ET")
stream = DecodedStreamObject()
stream.set_data("\n".join(ops).encode("latin-1"))
page[NameObject("/Contents")] = writer._add_object(stream)

if target_size > 4096:
    pad_stream = DecodedStreamObject()
    pad_stream.set_data(b"P" * (target_size - 2048))
    writer._add_object(pad_stream)

buf = io.BytesIO()
writer.write(buf)
raw = buf.getvalue()
if target_size > len(raw):
    rem = target_size - len(raw)
    if rem >= 2:
        raw = raw + b"\n%%" + (b"X" * (rem - 2))
    else:
        raw = raw + (b"\n" * rem)
sys.stdout.buffer.write(raw)
`, targetSize)
	cmd := exec.Command(env.Cfg.PythonBin, "-c", pyCode)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate PDF via python failed: %v (%s)", err, string(out))
	}
	return out
}

func generateValidSyllabusPDFBytes(t *testing.T, env *TestEnv) []byte {
	t.Helper()
	return generateValidSyllabusPDFBytesWithSize(t, env, 0)
}

func TestSelfContainedPythonParserParity(t *testing.T) {
	root := projectRootDir()
	origBytes, err := os.ReadFile(filepath.Join(root, "backend", "app", "services", "pdf_parser.py"))
	if err != nil {
		t.Skip("original backend/app/services/pdf_parser.py not present; pybridge is self-contained")
		return
	}
	bridgeBytes, err := os.ReadFile(filepath.Join(root, "backend-go", "pybridge", "pdf_parser.py"))
	if err != nil {
		t.Fatalf("read pybridge pdf_parser.py: %v", err)
	}
	normOrig := strings.ReplaceAll(string(origBytes), "\r\n", "\n")
	normBridge := strings.ReplaceAll(string(bridgeBytes), "\r\n", "\n")
	if normOrig != normBridge {
		t.Fatalf("backend-go/pybridge/pdf_parser.py does not match backend/app/services/pdf_parser.py")
	}
}

func TestPythonPDFBridgeEndToEnd(t *testing.T) {
	env := NewTestEnv(t)
	tokenA, _ := env.RegisterUser("pdf_a@example.com", "Password123!", "PDFA")
	tokenB, _ := env.RegisterUser("pdf_b@example.com", "Password123!", "PDFB")

	// 1. Reject non-PDF magic bytes -> 400 invalid_pdf
	code, errData, _ := env.DoMultipartPDF("/pdf/upload", "fake.pdf", []byte("NOT_A_PDF_FILE"), AuthHeader(tokenA))
	if code != http.StatusBadRequest || errData["code"] != "invalid_pdf" {
		t.Fatalf("expected 400 invalid_pdf, got %d %v", code, errData)
	}

	// 1b. Reject PDF exceeding 4.5 MB -> 400 pdf_too_large
	oversizedPDF := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("A"), int(env.Cfg.MaxPDFBytes)+1)...)
	code, errData, _ = env.DoMultipartPDF("/pdf/upload", "huge.pdf", oversizedPDF, AuthHeader(tokenA))
	if code != http.StatusBadRequest || errData["code"] != "pdf_too_large" {
		t.Fatalf("expected 400 pdf_too_large for >4.5 MB PDF, got %d %v", code, errData)
	}

	// 2. Upload real multi-line syllabus PDF -> parsed by existing Python pdf_parser.py via pybridge!
	pdfBytes := generateValidSyllabusPDFBytes(t, env)
	code, jobOut, rawBody := env.DoMultipartPDF("/pdf/upload", "networks_syllabus.pdf", pdfBytes, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("upload PDF failed: %d %s", code, string(rawBody))
	}
	if jobOut["status"] != "ready_for_preview" {
		t.Fatalf("expected status=ready_for_preview, got %v (error=%v)", jobOut["status"], jobOut["error_message"])
	}
	jobID, _ := jobOut["id"].(string)

	// 3. Cross-user isolation: User B cannot view User A's import job
	code, _, _ = env.DoJSON(http.MethodGet, "/pdf/jobs/"+jobID, nil, AuthHeader(tokenB))
	if code != http.StatusNotFound {
		t.Fatalf("User B GET User A job expected 404, got %d", code)
	}

	// 4. Patch import job tree before confirming
	code, patchedJob, _ := env.DoJSON(http.MethodPatch, "/pdf/jobs/"+jobID+"/tree", map[string]any{
		"generated_tree_json": map[string]any{
			"name": "Computer Networks",
			"children": []map[string]any{
				{
					"name": "Unit 1: Physical Layer",
					"children": []map[string]any{
						{"name": "Signal Propagation"},
						{"name": "Multiplexing"},
					},
				},
			},
		},
	}, AuthHeader(tokenA))
	if code != http.StatusOK || patchedJob["status"] != "ready_for_preview" {
		t.Fatalf("patch job tree failed: %d %v", code, patchedJob)
	}

	// 5. Confirm import job -> creates Skill and Topics in DB
	code, confirmedJob, _ := env.DoJSON(http.MethodPost, "/pdf/jobs/"+jobID+"/confirm", map[string]any{
		"skill_name":        "Computer Networks",
		"skill_description": "Imported from PDF",
	}, AuthHeader(tokenA))
	if code != http.StatusOK || confirmedJob["status"] != "confirmed" || confirmedJob["skill_id"] == nil {
		t.Fatalf("confirm job failed: %d %v", code, confirmedJob)
	}
}

func TestStorageBased10MBPDFUploadAndSecurity(t *testing.T) {
	env := NewTestEnv(t)
	tokenA, _ := env.RegisterUser("storage_a@example.com", "Password123!", "StorageA")
	tokenB, _ := env.RegisterUser("storage_b@example.com", "Password123!", "StorageB")

	// 1. Reject non-PDF extension or invalid MIME type on /pdf/upload-url
	code, errData, _ := env.DoJSON(http.MethodPost, "/pdf/upload-url", map[string]any{
		"filename":        "malicious.exe",
		"content_type":    "application/pdf",
		"file_size_bytes": 1024,
	}, AuthHeader(tokenA))
	if code != http.StatusBadRequest || errData["code"] != "invalid_pdf" {
		t.Fatalf("expected 400 invalid_pdf for .exe filename, got %d %v", code, errData)
	}

	code, errData, _ = env.DoJSON(http.MethodPost, "/pdf/upload-url", map[string]any{
		"filename":        "../traversal.pdf",
		"content_type":    "application/pdf",
		"file_size_bytes": 1024,
	}, AuthHeader(tokenA))
	if code != http.StatusBadRequest || errData["code"] != "invalid_pdf" {
		t.Fatalf("expected 400 invalid_pdf for path traversal filename, got %d %v", code, errData)
	}

	// 2. Reject > 4.5 MB (4,718,593 bytes) on /pdf/upload-url -> 400 pdf_too_large
	code, errData, _ = env.DoJSON(http.MethodPost, "/pdf/upload-url", map[string]any{
		"filename":        "over4_5mb.pdf",
		"content_type":    "application/pdf",
		"file_size_bytes": env.Cfg.MaxPDFBytes + 1,
	}, AuthHeader(tokenA))
	if code != http.StatusBadRequest || errData["code"] != "pdf_too_large" {
		t.Fatalf("expected 400 pdf_too_large for >4.5 MB on /pdf/upload-url, got %d %v", code, errData)
	}

	// 3. Test 1 MB PDF and 4.5 MB PDF (4,718,592 bytes) via signed storage upload flow
	for _, tc := range []struct {
		name      string
		sizeBytes int
		skillName string
	}{
		{name: "1MB_syllabus.pdf", sizeBytes: 1024 * 1024, skillName: "Networks 1MB"},
		{name: "4.5MB_syllabus.pdf", sizeBytes: int(env.Cfg.MaxPDFBytes), skillName: "Networks 4.5MB"},
	} {
		pdfBytes := generateValidSyllabusPDFBytesWithSize(t, env, tc.sizeBytes)
		if len(pdfBytes) != tc.sizeBytes {
			t.Fatalf("expected generated PDF to be %d bytes, got %d", tc.sizeBytes, len(pdfBytes))
		}

		// Step 1: Request signed upload URL
		code, urlOut, raw := env.DoJSON(http.MethodPost, "/pdf/upload-url", map[string]any{
			"filename":        tc.name,
			"content_type":    "application/pdf",
			"file_size_bytes": len(pdfBytes),
		}, AuthHeader(tokenA))
		if code != http.StatusOK {
			t.Fatalf("POST /pdf/upload-url (%s) failed: %d %s", tc.name, code, string(raw))
		}
		uploadURL, _ := urlOut["upload_url"].(string)
		objectKey, _ := urlOut["object_key"].(string)
		uploadToken, _ := urlOut["upload_token"].(string)

		// Security check: User B CANNOT upload to User A's signed URL (cross-user isolation)
		codeB, errB, _ := env.DoRawPut(uploadURL, "application/pdf", pdfBytes[:1024], AuthHeader(tokenB))
		if codeB != http.StatusForbidden {
			t.Fatalf("expected User B PUT to User A signed URL to fail with 403, got %d %v", codeB, errB)
		}

		// Step 2: User A uploads the PDF bytes directly to the signed URL
		codePut, putOut, rawPut := env.DoRawPut(uploadURL, "application/pdf", pdfBytes, AuthHeader(tokenA))
		if codePut != http.StatusOK || putOut["ok"] != true {
			t.Fatalf("PUT signed storage (%s) failed: %d %s", tc.name, codePut, string(rawPut))
		}

		// Security check: User B CANNOT call /pdf/upload-complete with User A's object_key
		codeCompB, _, _ := env.DoJSON(http.MethodPost, "/pdf/upload-complete", map[string]any{
			"object_key":        objectKey,
			"original_filename": tc.name,
			"file_size_bytes":   len(pdfBytes),
			"upload_token":      uploadToken,
		}, AuthHeader(tokenB))
		if codeCompB != http.StatusForbidden {
			t.Fatalf("expected User B /pdf/upload-complete on User A object_key to fail with 403, got %d", codeCompB)
		}

		// Security check: Tampered upload_token is rejected with 403
		codeTamper, _, _ := env.DoJSON(http.MethodPost, "/pdf/upload-complete", map[string]any{
			"object_key":        objectKey,
			"original_filename": tc.name,
			"file_size_bytes":   len(pdfBytes),
			"upload_token":      uploadToken + "tampered",
		}, AuthHeader(tokenA))
		if codeTamper != http.StatusForbidden {
			t.Fatalf("expected tampered upload_token to fail with 403, got %d", codeTamper)
		}

		// Step 3: User A completes the upload -> Python parses the 4.5 MB / 10 MB PDF and Go deletes the temporary storage object
		codeComp, jobOut, rawComp := env.DoJSON(http.MethodPost, "/pdf/upload-complete", map[string]any{
			"object_key":        objectKey,
			"original_filename": tc.name,
			"file_size_bytes":   len(pdfBytes),
			"upload_token":      uploadToken,
		}, AuthHeader(tokenA))
		if codeComp != http.StatusOK || jobOut["status"] != "ready_for_preview" {
			t.Fatalf("POST /pdf/upload-complete (%s) failed: %d %s", tc.name, codeComp, string(rawComp))
		}

		// Verify temporary storage file was automatically deleted after parsing
		storageFile := filepath.Join(env.Cfg.PDFStorageDir, filepath.FromSlash(objectKey))
		if _, statErr := os.Stat(storageFile); !os.IsNotExist(statErr) {
			t.Fatalf("expected temporary storage file %s to be deleted after processing", storageFile)
		}

		// Confirm import job
		jobID, _ := jobOut["id"].(string)
		codeConf, confOut, _ := env.DoJSON(http.MethodPost, "/pdf/jobs/"+jobID+"/confirm", map[string]any{
			"skill_name": tc.skillName,
		}, AuthHeader(tokenA))
		if codeConf != http.StatusOK || confOut["status"] != "confirmed" {
			t.Fatalf("confirm %s failed: %d %v", tc.name, codeConf, confOut)
		}
	}

	// 4. Reject SSRF / external URL in object_key -> 403 forbidden
	codeSSRF, errSSRF, _ := env.DoJSON(http.MethodPost, "/pdf/upload-complete", map[string]any{
		"object_key":        "https://evil.example.com/malicious.pdf",
		"original_filename": "malicious.pdf",
		"file_size_bytes":   1024,
		"upload_token":      "9999999999.deadbeef",
	}, AuthHeader(tokenA))
	if codeSSRF != http.StatusForbidden || errSSRF["code"] != "forbidden" {
		t.Fatalf("expected 403 forbidden for SSRF URL in object_key, got %d %v", codeSSRF, errSSRF)
	}

	// 5. Reject expired upload token -> 403 upload_expired
	var uidAStr string
	_ = env.DB.QueryRow(context.Background(), nil, `SELECT id FROM users WHERE email = 'storage_a@example.com'`).Scan(&uidAStr)
	uidA, _ := uuid.Parse(uidAStr)
	validKey := fmt.Sprintf("%s/%s/expired.pdf", uidA.String(), uuid.New().String())
	expiredToken, err := services.SignUploadToken(env.Cfg.EffectiveInternalSecret(), uidA, validKey, 1024, time.Now().UTC().Unix()-60)
	if err != nil {
		t.Fatalf("sign expired token: %v", err)
	}
	codeExp, errExp, _ := env.DoJSON(http.MethodPost, "/pdf/upload-complete", map[string]any{
		"object_key":        validKey,
		"original_filename": "expired.pdf",
		"file_size_bytes":   1024,
		"upload_token":      expiredToken,
	}, AuthHeader(tokenA))
	if codeExp != http.StatusForbidden || errExp["code"] != "upload_expired" {
		t.Fatalf("expected 403 upload_expired for expired token, got %d %v", codeExp, errExp)
	}

	// 6. Missing INTERNAL_PARSER_SECRET fails securely (503 parser_unconfigured on upload-url, and subprocess rejects unsigned input)
	origSecret := env.Cfg.InternalParserSecret
	env.Cfg.InternalParserSecret = ""
	codeNoSec, errNoSec, _ := env.DoJSON(http.MethodPost, "/pdf/upload-url", map[string]any{
		"filename":        "test.pdf",
		"content_type":    "application/pdf",
		"file_size_bytes": 1024,
	}, AuthHeader(tokenA))
	env.Cfg.InternalParserSecret = origSecret
	if codeNoSec != http.StatusServiceUnavailable || errNoSec["code"] != "parser_unconfigured" {
		t.Fatalf("expected 503 parser_unconfigured when INTERNAL_PARSER_SECRET is missing, got %d %v", codeNoSec, errNoSec)
	}

	// 7. Production mode MUST NOT fall back to local_signed if Supabase Storage is unconfigured
	origEnv := env.Cfg.Environment
	env.Cfg.Environment = "production"
	codeProdNoSupa, errProdNoSupa, _ := env.DoJSON(http.MethodPost, "/pdf/upload-url", map[string]any{
		"filename":        "prod.pdf",
		"content_type":    "application/pdf",
		"file_size_bytes": 1024,
	}, AuthHeader(tokenA))
	codeProdPut, errProdPut, _ := env.DoRawPut("/pdf/storage-upload?object_key=x&size=10&token=y", "application/pdf", []byte("%PDF-1.4"), AuthHeader(tokenA))
	env.Cfg.Environment = origEnv
	if codeProdNoSupa != http.StatusServiceUnavailable || errProdNoSupa["code"] != "storage_unconfigured" {
		t.Fatalf("expected 503 storage_unconfigured in production without Supabase Storage, got %d %v", codeProdNoSupa, errProdNoSupa)
	}
	if codeProdPut != http.StatusForbidden || errProdPut["code"] != "forbidden" {
		t.Fatalf("expected 403 forbidden on /pdf/storage-upload in production, got %d %v", codeProdPut, errProdPut)
	}
}

func TestSupabaseStorageAndRemotePythonParser10MB(t *testing.T) {
	env := NewTestEnv(t)
	token, _ := env.RegisterUser("supapdf@example.com", "Password123!", "SupaPDF")

	// Start a mock Supabase Storage server + real Python internal_pdf_parser HTTP handler
	var mu sync.Mutex
	objects := make(map[string][]byte)
	deletedKeys := make(map[string]bool)

	var supaServer *httptest.Server
	supaServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/storage/v1/object/upload/sign/"):
			key := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/upload/sign/pdf-imports/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"url": "/storage/v1/object/upload/sign/pdf-imports/" + key + "?token=mock-upload-token",
			})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/storage/v1/object/upload/sign/"):
			key := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/upload/sign/pdf-imports/")
			data, _ := io.ReadAll(r.Body)
			mu.Lock()
			objects[key] = data
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"Key":"` + key + `"}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/storage/v1/object/sign/"):
			key := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/sign/pdf-imports/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"signedURL": supaServer.URL + "/storage/v1/object/sign/pdf-imports/" + key + "?token=mock-download-token",
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/storage/v1/object/sign/"):
			key := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/sign/pdf-imports/")
			mu.Lock()
			data, ok := objects[key]
			mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/storage/v1/object/"):
			key := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/pdf-imports/")
			mu.Lock()
			delete(objects, key)
			deletedKeys[key] = true
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer supaServer.Close()

	// Start an httptest server that delegates to the actual Python api/internal_pdf_parser.py logic
	root := projectRootDir()
	apiDir := filepath.Join(root, "backend-go", "api")
	pyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		pyRunner := `
import io, os, sys
sys.path.insert(0, sys.argv[1])
import internal_pdf_parser

class DummySocket:
    def makefile(self, *args, **kwargs):
        return io.BytesIO(b"")

class CaptureHandler(internal_pdf_parser.handler):
    def __init__(self, headers_dict, body_bytes):
        self.headers = headers_dict
        self.rfile = io.BytesIO(body_bytes)
        self.wfile = io.BytesIO()
        self.status_code = 200
    def send_response(self, code, message=None):
        self.status_code = code
    def send_header(self, k, v):
        pass
    def end_headers(self):
        pass

raw_in = sys.stdin.buffer.read()
h = CaptureHandler({
    "Content-Type": os.environ.get("TEST_CT", ""),
    "Content-Length": str(len(raw_in)),
    "X-Internal-Bridge-Timestamp": os.environ.get("TEST_TS", ""),
    "X-Internal-Bridge-Signature": os.environ.get("TEST_SIG", ""),
}, raw_in)
h.do_POST()
sys.stdout.buffer.write(f"{h.status_code}\n".encode("utf-8") + h.wfile.getvalue())
`
		cmd := exec.Command(env.Cfg.PythonBin, "-c", pyRunner, apiDir)
		cmd.Stdin = bytes.NewReader(bodyBytes)
		cmd.Env = append(os.Environ(),
			"INTERNAL_PARSER_SECRET="+env.Cfg.EffectiveInternalSecret(),
			"SUPABASE_URL="+supaServer.URL,
			"MAX_PDF_BYTES="+strconv.FormatInt(env.Cfg.MaxPDFBytes, 10),
			"TEST_CT="+r.Header.Get("Content-Type"),
			"TEST_TS="+r.Header.Get("X-Internal-Bridge-Timestamp"),
			"TEST_SIG="+r.Header.Get("X-Internal-Bridge-Signature"),
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write(out)
			return
		}
		parts := bytes.SplitN(out, []byte("\n"), 2)
		stCode, _ := strconv.Atoi(strings.TrimSpace(string(parts[0])))
		if stCode == 0 {
			stCode = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(stCode)
		if len(parts) > 1 {
			_, _ = w.Write(parts[1])
		}
	}))
	defer pyServer.Close()

	// Verify unsigned / invalid signature requests to internal_pdf_parser.py are rejected with 403
	unauthReq, _ := http.NewRequest(http.MethodPost, pyServer.URL, bytes.NewReader([]byte(`{}`)))
	unauthReq.Header.Set("Content-Type", "application/json")
	unauthRes, err := http.DefaultClient.Do(unauthReq)
	if err != nil || unauthRes.StatusCode != http.StatusForbidden {
		t.Fatalf("expected unsigned request to internal_pdf_parser.py to return 403, got err=%v status=%d", err, unauthRes.StatusCode)
	}
	_ = unauthRes.Body.Close()

	// Configure env to use Supabase Storage + remote Python parser URL
	env.Cfg.SupabaseURL = supaServer.URL
	env.Cfg.SupabaseServiceRoleKey = "mock-service-role-key"
	env.Cfg.SupabaseStorageBucket = "pdf-imports"
	env.Cfg.PythonParserURL = pyServer.URL

	pdfPayload := generateValidSyllabusPDFBytesWithSize(t, env, int(env.Cfg.MaxPDFBytes))

	// Step 1: Request Supabase signed upload URL
	code, urlOut, _ := env.DoJSON(http.MethodPost, "/pdf/upload-url", map[string]any{
		"filename":        "supabase_4_5mb.pdf",
		"content_type":    "application/pdf",
		"file_size_bytes": len(pdfPayload),
	}, AuthHeader(token))
	if code != http.StatusOK || urlOut["storage_provider"] != "supabase" {
		t.Fatalf("expected supabase signed upload URL, got %d %v", code, urlOut)
	}
	uploadURL, _ := urlOut["upload_url"].(string)
	objectKey, _ := urlOut["object_key"].(string)
	uploadToken, _ := urlOut["upload_token"].(string)

	// Step 2: Frontend uploads PDF directly to Supabase Storage signed URL (bypassing Go server!)
	putReq, _ := http.NewRequest(http.MethodPut, uploadURL, bytes.NewReader(pdfPayload))
	putReq.Header.Set("Content-Type", "application/pdf")
	putRes, err := http.DefaultClient.Do(putReq)
	if err != nil || putRes.StatusCode != http.StatusOK {
		t.Fatalf("direct PUT to mock Supabase Storage failed: err=%v status=%v", err, putRes)
	}
	_ = putRes.Body.Close()

	// Step 3: Frontend notifies Go (/pdf/upload-complete) with small JSON body; Python fetches PDF from Supabase Storage and parses it; Go deletes object from Supabase Storage
	codeComp, jobOut, rawComp := env.DoJSON(http.MethodPost, "/pdf/upload-complete", map[string]any{
		"object_key":        objectKey,
		"original_filename": "supabase_4_5mb.pdf",
		"file_size_bytes":   len(pdfPayload),
		"upload_token":      uploadToken,
	}, AuthHeader(token))
	if codeComp != http.StatusOK || jobOut["status"] != "ready_for_preview" {
		t.Fatalf("supabase upload-complete failed: %d %s", codeComp, string(rawComp))
	}

	mu.Lock()
	wasDeleted := deletedKeys[objectKey]
	mu.Unlock()
	if !wasDeleted {
		t.Fatalf("expected Supabase Storage object %q to be deleted after processing", objectKey)
	}
}

func TestExistingDatabaseReadOnlyCopyCompatibility(t *testing.T) {
	root := projectRootDir()
	origDBPath := filepath.Join(root, "progress_tracker.db")
	if _, err := os.Stat(origDBPath); err != nil {
		origDBPath = filepath.Join(root, "backend-go", "progress_tracker.db")
		if _, err := os.Stat(origDBPath); err != nil {
			origDBPath = filepath.Join(root, "backend", "progress_tracker.db")
		}
	}
	info, err := os.Stat(origDBPath)
	if err != nil || info.Size() == 0 {
		t.Skip("progress_tracker.db not present; skipping existing DB regression test")
	}

	// Copy to temporary directory so we NEVER touch the original database file
	copyPath := filepath.Join(t.TempDir(), "progress_tracker_copy.db")
	srcFile, err := os.Open(origDBPath)
	if err != nil {
		t.Fatalf("open orig db: %v", err)
	}
	dstFile, err := os.Create(copyPath)
	if err != nil {
		_ = srcFile.Close()
		t.Fatalf("create copy db: %v", err)
	}
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		_ = srcFile.Close()
		_ = dstFile.Close()
		t.Fatalf("copy db: %v", err)
	}
	_ = srcFile.Close()
	_ = dstFile.Close()

	cfg := &config.Settings{
		DatabaseURL: "sqlite:///" + filepath.ToSlash(copyPath),
		Environment: "test",
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("open copied db: %v", err)
	}
	defer db.SQL.Close()

	ctx := context.Background()

	// 1. Verify all existing users and their Argon2id password hashes are valid PHC strings
	uRows, err := db.Query(ctx, nil, `SELECT id, email, password_hash FROM users`)
	if err != nil {
		t.Fatalf("query existing users: %v", err)
	}
	userCount := 0
	for uRows.Next() {
		var id, email string
		var pwdHash any
		if err := uRows.Scan(&id, &email, &pwdHash); err != nil {
			t.Fatalf("scan user: %v", err)
		}
		userCount++
		if ph := database.ParseNullString(pwdHash); ph != nil && *ph != "" {
			// Verify our Argon2 parser parses the existing argon2-cffi hash parameters without error
			if core.VerifyPassword("definitely-wrong-password", *ph) {
				t.Fatalf("unexpected match on wrong password for %s", email)
			}
		}
	}
	uRows.Close()
	if userCount == 0 {
		t.Fatalf("expected existing users in progress_tracker.db, got 0")
	}

	// 2. Verify all existing skills and topics load and build trees cleanly
	sRows, err := db.Query(ctx, nil, `SELECT `+handlers.SkillColumns+` FROM skills`)
	if err != nil {
		t.Fatalf("query existing skills: %v", err)
	}
	var skillIDs []uuid.UUID
	for sRows.Next() {
		sk, err := handlers.ScanSkill(sRows)
		if err != nil {
			t.Fatalf("scan existing skill: %v", err)
		}
		skillIDs = append(skillIDs, sk.ID)
	}
	sRows.Close()

	grouped, err := services.LoadUserTopicsGrouped(ctx, db, nil, skillIDs)
	if err != nil {
		t.Fatalf("LoadUserTopicsGrouped on existing DB: %v", err)
	}
	totalTopics := 0
	for _, sid := range skillIDs {
		topics := grouped[sid]
		totalTopics += len(topics)
		engine := services.NewProgressEngine(topics)
		tree := engine.BuildTree()
		comp, tot, pct := engine.SkillProgress()
		if tot < comp || pct < 0 || pct > 100.0 || tree == nil {
			t.Fatalf("invalid progress calculation on skill %s: comp=%d tot=%d pct=%f", sid, comp, tot, pct)
		}
	}
	if totalTopics < 100 {
		t.Fatalf("expected >100 topics in existing database, got %d", totalTopics)
	}
	t.Logf("Verified %d users, %d skills, and %d topics from existing database copy!", userCount, len(skillIDs), totalTopics)
}

func BenchmarkProgressEngine2000Topics(b *testing.B) {
	skillID := uuid.New()
	now := time.Now().UTC()
	topics := make([]models.Topic, 0, 2000)
	var roots []uuid.UUID
	for i := 0; i < 20; i++ {
		rid := uuid.New()
		roots = append(roots, rid)
		topics = append(topics, models.Topic{
			ID:         rid,
			SkillID:    skillID,
			Name:       fmt.Sprintf("Unit %d", i+1),
			OrderIndex: i,
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	for i := 20; i < 2000; i++ {
		pid := roots[i%len(roots)]
		topics = append(topics, models.Topic{
			ID:          uuid.New(),
			SkillID:     skillID,
			ParentID:    &pid,
			Name:        fmt.Sprintf("Subtopic %d", i),
			OrderIndex:  i,
			IsCompleted: i%2 == 0,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine := services.NewProgressEngine(topics)
		_ = engine.BuildTree()
		_, _, _ = engine.SkillProgress()
	}
}

func BenchmarkExtractAndRepairJSON(b *testing.B) {
	raw := "```json\n" + `{"skill_name":"System Design","topics":[{"name":"Load Balancing","children":[{"name":"L4 vs L7"},{"name":"Consistent Hashing"}]},{"name":"Caching","children":[{"name":"Write-Through"},{"name":"Write-Back"},]}]}` + "\n```"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := services.ExtractJSON(raw)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDashboardEndpoint(b *testing.B) {
	tmpDB := filepath.Join(b.TempDir(), "bench.db")
	cfg := &config.Settings{
		DatabaseURL:              "sqlite:///" + filepath.ToSlash(tmpDB),
		JWTSecret:                "bench-jwt-secret-key-with-32-bytes-minimum!",
		JWTRefreshSecret:         "bench-ref-secret-key-with-32-bytes-minimum!",
		AccessTokenExpireMinutes: 30,
		RefreshTokenExpireDays:   30,
		Environment:              "test",
	}
	db, err := database.Open(cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer db.SQL.Close()
	limiter := core.NewRateLimiter(cfg)
	limiter.SetEnabled(false)
	router := handlers.NewRouter(cfg, db, limiter)

	regBody := []byte(`{"email":"bench@example.com","password":"Password123!","name":"Bench","timezone":"UTC"}`)
	regReq := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regRec := httptest.NewRecorder()
	router.ServeHTTP(regRec, regReq)

	token, _ := core.CreateAccessToken(cfg, "", nil, nil)
	var uid string
	_ = db.QueryRow(context.Background(), nil, `SELECT id FROM users LIMIT 1`).Scan(&uid)
	token, _ = core.CreateAccessToken(cfg, uid, nil, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status=%d", rec.Code)
		}
	}
}
