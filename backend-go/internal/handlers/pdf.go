package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/models"
	"skill-progress-tracker/backend-go/internal/services"
)

const uploadURLExpirySeconds = 300

func serializeImportJob(job *models.ImportJob) models.ImportJobOut {
	return models.ImportJobOut{
		ID:                job.ID,
		Status:            string(job.Status),
		OriginalFilename:  job.OriginalFilename,
		FileSizeBytes:     job.FileSizeBytes,
		SkillID:           job.SkillID,
		GeneratedTreeJSON: job.GeneratedTreeJSON,
		ConfidenceNotes:   job.ConfidenceNotes,
		ErrorMessage:      job.ErrorMessage,
	}
}

func (s *Server) loadOwnedJob(ctx context.Context, jobID uuid.UUID, user *models.User) (*models.ImportJob, *core.AppError) {
	row := s.DB.QueryRow(
		ctx, nil,
		`SELECT `+ImportJobColumns+` FROM import_jobs WHERE id = ? AND user_id = ?`,
		s.DB.FormatUUID(jobID), s.DB.FormatUUID(user.ID),
	)
	job, err := ScanImportJob(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, core.NewAppError(http.StatusNotFound, "not_found", "Import job not found.")
		}
		return nil, core.NewAppError(http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
	}
	return job, nil
}

func (s *Server) localStorageFilePath(objectKey string) (string, error) {
	baseDir, err := filepath.Abs(s.Cfg.PDFStorageDir)
	if err != nil {
		return "", err
	}
	fullPath := filepath.Clean(filepath.Join(baseDir, filepath.FromSlash(objectKey)))
	rel, err := filepath.Rel(baseDir, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", core.NewAppError(http.StatusForbidden, "forbidden", "Invalid storage path.")
	}
	return fullPath, nil
}

func (s *Server) cleanupStorageObject(ctx context.Context, objectKey string) {
	if s.Cfg.HasSupabaseStorage() {
		_ = services.DeleteSupabaseStorageObject(ctx, s.Cfg, objectKey)
		return
	}
	if fullPath, err := s.localStorageFilePath(objectKey); err == nil {
		_ = os.Remove(fullPath)
	}
}

// CreatePDFUploadURL issues a short-lived signed upload URL scoped to the authenticated user.
// Step 1 of the storage-based 10 MB PDF upload flow.
func (s *Server) CreatePDFUploadURL(w http.ResponseWriter, r *http.Request, user *models.User) {
	if appErr := s.Limiter.EnforceLimit(s.Cfg, r, "pdf_upload", 10, 3600); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	body, ok := DecodeBody[models.PDFUploadURLIn](w, r)
	if !ok {
		return
	}

	safeName, err := services.SanitizePDFObjectFilename(body.Filename)
	if err != nil {
		if ae, ok := err.(*core.AppError); ok {
			core.WriteAppError(w, ae)
		} else {
			core.WriteError(w, http.StatusBadRequest, "invalid_pdf", "Only .pdf files are supported.")
		}
		return
	}

	ct := strings.ToLower(strings.TrimSpace(body.ContentType))
	if ct != "" && ct != "application/pdf" && ct != "application/x-pdf" {
		core.WriteError(w, http.StatusBadRequest, "invalid_pdf", "File is not a valid PDF (content-type/magic mismatch).")
		return
	}

	if body.FileSizeBytes > s.Cfg.MaxPDFBytes {
		core.WriteError(w, http.StatusBadRequest, "pdf_too_large", "PDF exceeds the 4.5 MB limit.")
		return
	}

	secret := s.Cfg.EffectiveInternalSecret()
	if secret == "" {
		core.WriteError(w, http.StatusServiceUnavailable, "parser_unconfigured", "Internal PDF parser secret is not configured.")
		return
	}

	uploadID := uuid.New()
	objectKey := fmt.Sprintf("%s/%s/%s", user.ID.String(), uploadID.String(), safeName)
	expiresAt := time.Now().UTC().Unix() + uploadURLExpirySeconds

	uploadToken, err := services.SignUploadToken(secret, user.ID, objectKey, body.FileSizeBytes, expiresAt)
	if err != nil {
		core.WriteError(w, http.StatusServiceUnavailable, "parser_unconfigured", "Internal PDF parser secret is not configured.")
		return
	}

	if s.Cfg.HasSupabaseStorage() {
		signedURL, err := services.CreateSupabaseSignedUploadURL(r.Context(), s.Cfg, objectKey)
		if err != nil {
			core.Logger.Error("Failed to create Supabase signed upload URL", "error", err)
			core.WriteError(w, http.StatusBadGateway, "storage_error", "Could not initialize private PDF storage upload.")
			return
		}
		core.WriteJSON(w, http.StatusOK, models.PDFUploadURLOut{
			UploadURL:       signedURL,
			ObjectKey:       objectKey,
			UploadToken:     uploadToken,
			StorageProvider: "supabase",
			ExpiresIn:       uploadURLExpirySeconds,
			MaxBytes:        s.Cfg.MaxPDFBytes,
		})
		return
	}

	if !s.Cfg.AllowsLocalSignedPDFStorage() {
		core.WriteError(w, http.StatusServiceUnavailable, "storage_unconfigured", "Private PDF storage (Supabase Storage) is required in production.")
		return
	}

	localUploadURL := fmt.Sprintf(
		"/pdf/storage-upload?object_key=%s&size=%d&token=%s",
		url.QueryEscape(objectKey),
		body.FileSizeBytes,
		url.QueryEscape(uploadToken),
	)
	core.WriteJSON(w, http.StatusOK, models.PDFUploadURLOut{
		UploadURL:       localUploadURL,
		ObjectKey:       objectKey,
		UploadToken:     uploadToken,
		StorageProvider: "local_signed",
		ExpiresIn:       uploadURLExpirySeconds,
		MaxBytes:        s.Cfg.MaxPDFBytes,
	})
}

// UploadToSignedStorage receives raw PDF bytes for a signed local/test private storage URL.
// Step 2 of the storage-based 10 MB PDF upload flow when Supabase Storage is not configured.
// Strictly disabled in production and serverless environments.
func (s *Server) UploadToSignedStorage(w http.ResponseWriter, r *http.Request, user *models.User) {
	if !s.Cfg.AllowsLocalSignedPDFStorage() || s.Cfg.HasSupabaseStorage() {
		core.WriteError(w, http.StatusForbidden, "forbidden", "Local signed storage uploads are disabled in production.")
		return
	}

	q := r.URL.Query()
	objectKey := strings.TrimSpace(q.Get("object_key"))
	sizeStr := strings.TrimSpace(q.Get("size"))
	token := strings.TrimSpace(q.Get("token"))

	if err := services.ValidateOwnedObjectKey(objectKey, user.ID); err != nil {
		if ae, ok := err.(*core.AppError); ok {
			core.WriteAppError(w, ae)
		} else {
			core.WriteError(w, http.StatusForbidden, "forbidden", "Invalid storage object key.")
		}
		return
	}

	declaredSize, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil || declaredSize <= 0 {
		core.WriteError(w, http.StatusBadRequest, "invalid_pdf", "Invalid declared PDF size.")
		return
	}
	if declaredSize > s.Cfg.MaxPDFBytes {
		core.WriteError(w, http.StatusBadRequest, "pdf_too_large", "PDF exceeds the 4.5 MB limit.")
		return
	}

	if err := services.VerifyUploadToken(s.Cfg.EffectiveInternalSecret(), token, user.ID, objectKey, declaredSize, time.Now().UTC().Unix()); err != nil {
		if ae, ok := err.(*core.AppError); ok {
			core.WriteAppError(w, ae)
		} else {
			core.WriteError(w, http.StatusForbidden, "forbidden", "Invalid or expired upload token.")
		}
		return
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, s.Cfg.MaxPDFBytes+1))
	if err != nil {
		core.WriteValidationError(w)
		return
	}
	if int64(len(raw)) > s.Cfg.MaxPDFBytes {
		core.WriteError(w, http.StatusBadRequest, "pdf_too_large", "PDF exceeds the 4.5 MB limit.")
		return
	}
	if int64(len(raw)) != declaredSize {
		core.WriteError(w, http.StatusBadRequest, "invalid_pdf", "Uploaded file size does not match declared size.")
		return
	}
	if !bytes.HasPrefix(raw, []byte("%PDF")) {
		core.WriteError(w, http.StatusBadRequest, "invalid_pdf", "File is not a valid PDF (content-type/magic mismatch).")
		return
	}

	fullPath, err := s.localStorageFilePath(objectKey)
	if err != nil {
		core.WriteError(w, http.StatusForbidden, "forbidden", "Invalid storage object key.")
		return
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	if err := os.WriteFile(fullPath, raw, 0o600); err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	core.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"object_key": objectKey,
	})
}

// CompletePDFUpload verifies ownership of the uploaded storage object, invokes the Python
// PDF parser against private storage, persists the ImportJob, and deletes the temporary PDF.
// Step 3 of the storage-based 10 MB PDF upload flow.
func (s *Server) CompletePDFUpload(w http.ResponseWriter, r *http.Request, user *models.User) {
	body, ok := DecodeBody[models.PDFUploadCompleteIn](w, r)
	if !ok {
		return
	}

	if err := services.ValidateOwnedObjectKey(body.ObjectKey, user.ID); err != nil {
		if ae, ok := err.(*core.AppError); ok {
			core.WriteAppError(w, ae)
		} else {
			core.WriteError(w, http.StatusForbidden, "forbidden", "Invalid storage object key.")
		}
		return
	}

	if body.FileSizeBytes > s.Cfg.MaxPDFBytes {
		s.cleanupStorageObject(r.Context(), body.ObjectKey)
		core.WriteError(w, http.StatusBadRequest, "pdf_too_large", "PDF exceeds the 4.5 MB limit.")
		return
	}

	if err := services.VerifyUploadToken(s.Cfg.EffectiveInternalSecret(), body.UploadToken, user.ID, body.ObjectKey, body.FileSizeBytes, time.Now().UTC().Unix()); err != nil {
		if ae, ok := err.(*core.AppError); ok {
			core.WriteAppError(w, ae)
		} else {
			core.WriteError(w, http.StatusForbidden, "forbidden", "Invalid or expired upload token.")
		}
		return
	}

	// Ensure temporary PDF object is always deleted from private storage after parsing or failure.
	defer s.cleanupStorageObject(context.Background(), body.ObjectKey)

	filename := filepath.Base(strings.TrimSpace(body.OriginalFilename))
	if filename == "" || filename == "." || filename == "/" {
		filename = "upload.pdf"
	}
	if runes := []rune(filename); len(runes) > 500 {
		filename = string(runes[:500])
	}

	ctx := r.Context()

	if s.Cfg.HasSupabaseStorage() {
		if s.Cfg.EffectivePythonParserURL() == "" {
			core.WriteError(w, http.StatusServiceUnavailable, "parser_unconfigured", "Internal PDF parser URL is not configured.")
			return
		}
		downloadURL, err := services.CreateSupabaseSignedDownloadURL(ctx, s.Cfg, body.ObjectKey, 60)
		if err != nil {
			core.Logger.Error("Failed to create Supabase signed download URL", "error", err)
			core.WriteError(w, http.StatusBadGateway, "storage_error", "Could not access uploaded PDF in private storage.")
			return
		}
		rawTree, errCode, errMsg := services.InvokePythonPDFParserFromStorageURL(ctx, s.Cfg, downloadURL, body.ObjectKey)
		if errCode == "pdf_too_large" || errCode == "invalid_pdf" {
			core.WriteError(w, http.StatusBadRequest, errCode, errMsg)
			return
		}
		job, appErr := s.insertImportJob(ctx, user.ID, filename, int(body.FileSizeBytes))
		if appErr != nil {
			core.WriteAppError(w, appErr)
			return
		}
		s.finalizeParsedJob(ctx, w, job, rawTree, errMsg)
		return
	}

	if !s.Cfg.AllowsLocalSignedPDFStorage() {
		core.WriteError(w, http.StatusServiceUnavailable, "storage_unconfigured", "Private PDF storage (Supabase Storage) is required in production.")
		return
	}

	// Local/test signed storage path
	fullPath, err := s.localStorageFilePath(body.ObjectKey)
	if err != nil {
		core.WriteError(w, http.StatusForbidden, "forbidden", "Invalid storage object key.")
		return
	}
	raw, err := os.ReadFile(fullPath)
	if err != nil {
		core.WriteError(w, http.StatusNotFound, "not_found", "Uploaded PDF object was not found or has already been processed.")
		return
	}
	if int64(len(raw)) > s.Cfg.MaxPDFBytes {
		core.WriteError(w, http.StatusBadRequest, "pdf_too_large", "PDF exceeds the 4.5 MB limit.")
		return
	}
	if !bytes.HasPrefix(raw, []byte("%PDF")) {
		core.WriteError(w, http.StatusBadRequest, "invalid_pdf", "File is not a valid PDF (content-type/magic mismatch).")
		return
	}

	job, appErr := s.insertImportJob(ctx, user.ID, filename, len(raw))
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	rawTree, errMsg := services.InvokePythonPDFParser(ctx, s.Cfg, raw)
	s.finalizeParsedJob(ctx, w, job, rawTree, errMsg)
}

func (s *Server) insertImportJob(ctx context.Context, userID uuid.UUID, filename string, fileSizeBytes int) (*models.ImportJob, *core.AppError) {
	now := time.Now().UTC()
	job := &models.ImportJob{
		ID:               uuid.New(),
		UserID:           userID,
		OriginalFilename: filename,
		FileSizeBytes:    fileSizeBytes,
		Status:           models.StatusUploaded,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	_, err := s.DB.Exec(
		ctx, nil,
		`INSERT INTO import_jobs (id, user_id, skill_id, original_filename, file_size_bytes, status, confidence_notes, error_message, generated_tree_json, created_at, updated_at)
		 VALUES (?, ?, NULL, ?, ?, ?, NULL, NULL, NULL, ?, ?)`,
		s.DB.FormatUUID(job.ID),
		s.DB.FormatUUID(userID),
		job.OriginalFilename,
		job.FileSizeBytes,
		string(job.Status),
		s.DB.FormatTime(now),
		s.DB.FormatTime(now),
	)
	if err != nil {
		return nil, core.NewAppError(http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
	}
	return job, nil
}

func (s *Server) finalizeParsedJob(ctx context.Context, w http.ResponseWriter, job *models.ImportJob, rawTree map[string]any, errMsg string) {
	now := time.Now().UTC()
	if errMsg != "" || rawTree == nil {
		if errMsg == "" {
			errMsg = "Parsing failed. The file was not stored as a skill."
		}
		job.Status = models.StatusFailed
		job.ErrorMessage = &errMsg
		job.UpdatedAt = now
		_, _ = s.DB.Exec(
			ctx, nil,
			`UPDATE import_jobs SET status = ?, error_message = ?, updated_at = ? WHERE id = ?`,
			string(job.Status), errMsg, s.DB.FormatTime(now), s.DB.FormatUUID(job.ID),
		)
		core.WriteJSON(w, http.StatusOK, serializeImportJob(job))
		return
	}

	if budgetErr := services.AssertPDFTreeBudget(rawTree); budgetErr != nil {
		msg := "Import tree has too many topics."
		if ae, ok := budgetErr.(*core.AppError); ok {
			msg = ae.Message
		}
		job.Status = models.StatusFailed
		job.ErrorMessage = &msg
		job.UpdatedAt = now
		_, _ = s.DB.Exec(
			ctx, nil,
			`UPDATE import_jobs SET status = ?, error_message = ?, updated_at = ? WHERE id = ?`,
			string(job.Status), msg, s.DB.FormatTime(now), s.DB.FormatUUID(job.ID),
		)
		core.WriteJSON(w, http.StatusOK, serializeImportJob(job))
		return
	}

	sanitizedChildren, sanErr := services.SanitizePDFNodes(rawTree["children"], 0)
	if sanErr != nil {
		msg := "Import tree exceeds the maximum depth."
		if ae, ok := sanErr.(*core.AppError); ok {
			msg = ae.Message
		}
		job.Status = models.StatusFailed
		job.ErrorMessage = &msg
		job.UpdatedAt = now
		_, _ = s.DB.Exec(
			ctx, nil,
			`UPDATE import_jobs SET status = ?, error_message = ?, updated_at = ? WHERE id = ?`,
			string(job.Status), msg, s.DB.FormatTime(now), s.DB.FormatUUID(job.ID),
		)
		core.WriteJSON(w, http.StatusOK, serializeImportJob(job))
		return
	}

	rawTree["children"] = sanitizedChildren
	confNotes := map[string]any{
		"low_confidence_sections": rawTree["low_confidence_sections"],
	}
	if confNotes["low_confidence_sections"] == nil {
		confNotes["low_confidence_sections"] = []any{}
	}
	if cnMap, ok := rawTree["confidence_notes"].(map[string]any); ok {
		for k, v := range cnMap {
			confNotes[k] = v
		}
	}

	job.GeneratedTreeJSON = rawTree
	job.ConfidenceNotes = confNotes
	job.Status = models.StatusReadyForPreview
	job.UpdatedAt = now

	_, err := s.DB.Exec(
		ctx, nil,
		`UPDATE import_jobs SET status = ?, generated_tree_json = ?, confidence_notes = ?, updated_at = ? WHERE id = ?`,
		string(job.Status),
		MarshalJSONString(job.GeneratedTreeJSON),
		MarshalJSONString(job.ConfidenceNotes),
		s.DB.FormatTime(now),
		s.DB.FormatUUID(job.ID),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	core.WriteJSON(w, http.StatusOK, serializeImportJob(job))
}

func (s *Server) UploadPDF(w http.ResponseWriter, r *http.Request, user *models.User) {
	if appErr := s.Limiter.EnforceLimit(s.Cfg, r, "pdf_upload", 10, 3600); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	if clStr := strings.TrimSpace(r.Header.Get("Content-Length")); clStr != "" {
		if cl, err := strconv.ParseInt(clStr, 10, 64); err == nil {
			if cl > s.Cfg.MaxPDFBytes+65536 {
				core.WriteError(w, http.StatusBadRequest, "pdf_too_large", "PDF exceeds the 4.5 MB limit.")
				return
			}
		}
	}

	// Limit reader to prevent memory exhaustion or server crashes if someone streams massive files (e.g. 1 TB).
	r.Body = http.MaxBytesReader(w, r.Body, s.Cfg.MaxPDFBytes+131072)

	if err := r.ParseMultipartForm(s.Cfg.MaxPDFBytes + 65536); err != nil {
		core.WriteError(w, http.StatusBadRequest, "pdf_too_large", "PDF exceeds the 4.5 MB limit.")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		core.WriteValidationError(w)
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(io.LimitReader(file, s.Cfg.MaxPDFBytes+1))
	if err != nil {
		core.WriteValidationError(w)
		return
	}
	if int64(len(raw)) > s.Cfg.MaxPDFBytes {
		core.WriteError(w, http.StatusBadRequest, "pdf_too_large", "PDF exceeds the 4.5 MB limit.")
		return
	}
	if !bytes.HasPrefix(raw, []byte("%PDF")) {
		core.WriteError(w, http.StatusBadRequest, "invalid_pdf", "File is not a valid PDF (content-type/magic mismatch).")
		return
	}

	filename := "upload.pdf"
	if header != nil && strings.TrimSpace(header.Filename) != "" {
		filename = filepath.Base(header.Filename)
	}
	if runes := []rune(filename); len(runes) > 500 {
		filename = string(runes[:500])
	}

	ctx := r.Context()
	job, appErr := s.insertImportJob(ctx, user.ID, filename, len(raw))
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}

	rawTree, errMsg := services.InvokePythonPDFParser(ctx, s.Cfg, raw)
	s.finalizeParsedJob(ctx, w, job, rawTree, errMsg)
}

func (s *Server) GetJob(w http.ResponseWriter, r *http.Request, user *models.User) {
	jobID, ok := ParsePathUUID(w, r.PathValue("job_id"))
	if !ok {
		return
	}
	job, appErr := s.loadOwnedJob(r.Context(), jobID, user)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	core.WriteJSON(w, http.StatusOK, serializeImportJob(job))
}

func (s *Server) PatchJob(w http.ResponseWriter, r *http.Request, user *models.User) {
	jobID, ok := ParsePathUUID(w, r.PathValue("job_id"))
	if !ok {
		return
	}
	body, ok := DecodeBody[models.TreePatchIn](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	job, appErr := s.loadOwnedJob(ctx, jobID, user)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	if job.Status != models.StatusReadyForPreview && job.Status != models.StatusParsing {
		core.WriteError(w, http.StatusConflict, "not_editable", "This import job cannot be edited in its current state.")
		return
	}
	sanitized, err := services.SanitizePDFTree(body.GeneratedTreeJSON)
	if err != nil {
		if ae, ok := err.(*core.AppError); ok {
			core.WriteAppError(w, ae)
		} else {
			core.WriteError(w, http.StatusBadRequest, "invalid_tree", "Import tree is invalid.")
		}
		return
	}
	now := time.Now().UTC()
	job.GeneratedTreeJSON = sanitized
	job.UpdatedAt = now

	_, err = s.DB.Exec(
		ctx, nil,
		`UPDATE import_jobs SET generated_tree_json = ?, updated_at = ? WHERE id = ?`,
		MarshalJSONString(job.GeneratedTreeJSON),
		s.DB.FormatTime(now),
		s.DB.FormatUUID(job.ID),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, serializeImportJob(job))
}

func (s *Server) ConfirmJob(w http.ResponseWriter, r *http.Request, user *models.User) {
	jobID, ok := ParsePathUUID(w, r.PathValue("job_id"))
	if !ok {
		return
	}
	body, ok := DecodeBody[models.ConfirmImportIn](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	job, appErr := s.loadOwnedJob(ctx, jobID, user)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	if job.Status == models.StatusConfirmed && job.SkillID != nil {
		core.WriteJSON(w, http.StatusOK, serializeImportJob(job))
		return
	}
	if job.Status != models.StatusReadyForPreview {
		core.WriteError(w, http.StatusConflict, "not_ready", "Import is not ready to confirm.")
		return
	}
	treeMap := job.GeneratedTreeJSON
	if treeMap == nil {
		treeMap = map[string]any{}
	}
	tree, err := services.SanitizePDFTree(treeMap)
	if err != nil {
		if ae, ok := err.(*core.AppError); ok {
			core.WriteAppError(w, ae)
		} else {
			core.WriteError(w, http.StatusBadRequest, "invalid_tree", "Import tree is invalid.")
		}
		return
	}
	if err := services.AssertPDFTreeBudget(tree); err != nil {
		if ae, ok := err.(*core.AppError); ok {
			core.WriteAppError(w, ae)
		} else {
			core.WriteError(w, http.StatusBadRequest, "invalid_tree", "Import tree has too many topics.")
		}
		return
	}

	stem := strings.TrimSuffix(job.OriginalFilename, filepath.Ext(job.OriginalFilename))
	if stem == "" {
		stem = "Imported skill"
	}
	skillName := ""
	isCustomName := false
	if body.SkillName != nil && strings.TrimSpace(*body.SkillName) != "" {
		skillName = strings.TrimSpace(*body.SkillName)
		isCustomName = true
	} else if stem != "Imported skill" && stem != "" {
		skillName = stem
	} else if tName, ok := tree["name"].(string); ok && strings.TrimSpace(tName) != "" && strings.TrimSpace(tName) != "Imported syllabus" {
		skillName = strings.TrimSpace(tName)
	} else {
		skillName = stem
	}
	if runes := []rune(skillName); len(runes) > 200 {
		skillName = string(runes[:200])
	}

	var existingID string
	err = s.DB.QueryRow(
		ctx, nil,
		`SELECT id FROM skills WHERE user_id = ? AND name = ?`,
		s.DB.FormatUUID(user.ID), skillName,
	).Scan(&existingID)

	if err == nil && !isCustomName {
		// Default name already exists — automatically make it unique so default import never fails
		baseName := skillName
		for i := 2; i <= 1000; i++ {
			candidate := fmt.Sprintf("%s (%d)", baseName, i)
			if len([]rune(candidate)) > 200 {
				candidate = fmt.Sprintf("%.190s (%d)", baseName, i)
			}
			var dummy string
			checkErr := s.DB.QueryRow(ctx, nil, `SELECT id FROM skills WHERE user_id = ? AND name = ?`, s.DB.FormatUUID(user.ID), candidate).Scan(&dummy)
			if checkErr != nil {
				skillName = candidate
				err = checkErr
				break
			}
		}
	}

	if err == nil {
		core.WriteError(w, http.StatusConflict, "skill_exists", "You already have a skill with this name. Please enter a different skill name.")
		return
	}

	skillID := uuid.New()
	now := time.Now().UTC()
	children, _ := services.SanitizePDFNodes(tree["children"], 0)
	summary := fmt.Sprintf("You imported %q and created %q", job.OriginalFilename, skillName)

	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.DB.Exec(
			ctx, tx,
			`INSERT INTO skills (id, user_id, name, description, deleted_at, purge_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, NULL, NULL, ?, ?)`,
			s.DB.FormatUUID(skillID),
			s.DB.FormatUUID(user.ID),
			skillName,
			body.SkillDescription,
			s.DB.FormatTime(now),
			s.DB.FormatTime(now),
		); err != nil {
			return err
		}
		if _, err := services.PersistTopicNodes(ctx, s.DB, tx, skillID, children, nil); err != nil {
			return err
		}
		if _, err := s.DB.Exec(
			ctx, tx,
			`UPDATE import_jobs SET skill_id = ?, status = ?, updated_at = ? WHERE id = ?`,
			s.DB.FormatUUID(skillID),
			string(models.StatusConfirmed),
			s.DB.FormatTime(now),
			s.DB.FormatUUID(job.ID),
		); err != nil {
			return err
		}
		sid := skillID
		sname := skillName
		tname := job.OriginalFilename
		return services.LogActivity(ctx, s.DB, tx, user, models.ActionPDFImported, nil, &sid, &tname, &sname, nil, &summary)
	})
	if err != nil {
		if database.IsUniqueViolation(err) {
			core.WriteError(w, http.StatusConflict, "skill_exists", "You already have a skill with this name.")
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	job.SkillID = &skillID
	job.Status = models.StatusConfirmed
	job.UpdatedAt = now
	core.WriteJSON(w, http.StatusOK, serializeImportJob(job))
}
