import { apiClient } from './client';

// apiClient is imported so the token is initialized via its module side-effect.
void apiClient;

export async function uploadFile(file: File): Promise<string> {
  const formData = new FormData();
  formData.append('file', file);

  // Use fetch directly since apiClient uses JSON content-type headers
  const token = localStorage.getItem('token');
  const response = await fetch('/api/upload', {
    method: 'POST',
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    body: formData,
  });

  if (!response.ok) {
    throw new Error(`Upload failed: ${response.statusText}`);
  }

  const data = await response.json();
  return data.url as string;
}
