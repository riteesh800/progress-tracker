import { ApiError, api } from "./client";

export type ImportJob = {
  id: string;
  status: string;
  original_filename: string;
  file_size_bytes: number;
  skill_id?: string | null;
  generated_tree_json?: ProposedNode | null;
  confidence_notes?: unknown;
  error_message?: string | null;
};

export type ProposedNode = {
  name: string;
  confidence?: string;
  source?: string;
  children?: ProposedNode[];
};

export type PDFUploadURLResponse = {
  upload_url: string;
  object_key: string;
  upload_token: string;
  storage_provider: "supabase" | "local_signed";
  expires_in: number;
  max_bytes: number;
};

export async function uploadPdf(file: File): Promise<ImportJob> {
  // Step 1: Request a short-lived user-scoped signed upload URL from the Go backend
  const signed = await api<PDFUploadURLResponse>("/pdf/upload-url", {
    method: "POST",
    body: JSON.stringify({
      filename: file.name || "upload.pdf",
      content_type: file.type || "application/pdf",
      file_size_bytes: file.size,
    }),
  });

  // Step 2: Upload the PDF directly to private object storage (bypasses Vercel 4.5 MB function payload limit)
  if (signed.storage_provider === "supabase") {
    const putRes = await fetch(signed.upload_url, {
      method: "PUT",
      headers: {
        "Content-Type": file.type || "application/pdf",
      },
      body: file,
    });
    if (!putRes.ok) {
      throw new ApiError(putRes.status, "Failed to upload PDF to private storage.");
    }
  } else {
    await api<{ ok: boolean }>(signed.upload_url, {
      method: "PUT",
      headers: {
        "Content-Type": file.type || "application/pdf",
      },
      body: file,
      timeoutMs: 120000,
    });
  }

  // Step 3: Notify the Go backend to parse the uploaded storage object and create the import job
  return api<ImportJob>("/pdf/upload-complete", {
    method: "POST",
    body: JSON.stringify({
      object_key: signed.object_key,
      original_filename: file.name || "upload.pdf",
      file_size_bytes: file.size,
      upload_token: signed.upload_token,
    }),
    timeoutMs: 120000,
  });
}

export const getJob = (id: string) => api<ImportJob>(`/pdf/jobs/${id}`);
export const patchTree = (id: string, generated_tree_json: ProposedNode) =>
  api<ImportJob>(`/pdf/jobs/${id}/tree`, { method: "PATCH", body: JSON.stringify({ generated_tree_json }) });
export const confirmJob = (id: string, skill_name?: string) =>
  api<ImportJob>(`/pdf/jobs/${id}/confirm`, { method: "POST", body: JSON.stringify({ skill_name }) });
