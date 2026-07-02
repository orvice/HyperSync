interface UploadResult {
  id: string;
  cdn_url: string;
  s3_key: string;
  content_type: string;
  size_bytes: number;
}

export async function uploadMedia(file: File): Promise<UploadResult> {
  const formData = new FormData();
  formData.append("file", file);

  const token = localStorage.getItem("token");
  const resp = await fetch("/api/media/upload", {
    method: "POST",
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    body: formData,
  });

  if (!resp.ok) {
    throw new Error("Upload failed");
  }

  return resp.json();
}
