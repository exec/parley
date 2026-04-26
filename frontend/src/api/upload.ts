import { apiClient, apiUrl } from './client';

// apiClient is imported so the token is initialized via its module side-effect.
void apiClient;

export async function uploadFile(file: File): Promise<string> {
  const formData = new FormData();
  formData.append('file', file);

  // Use fetch directly since apiClient sets Content-Type: application/json
  // which conflicts with the multipart boundary the browser computes for us.
  const token = apiClient.getToken();
  const response = await fetch(apiUrl('/upload'), {
    method: 'POST',
    credentials: 'include',
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    body: formData,
  });

  if (!response.ok) {
    throw new Error(`Upload failed: ${response.statusText}`);
  }

  const data = await response.json();
  return data.url as string;
}
