export function getAuthHeaders(): Record<string, string> {
  const headers: Record<string, string> = {};
  const token = localStorage.getItem('auth_token');
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }

  const apiKey = localStorage.getItem('api_key') || process.env.REACT_APP_API_KEY;
  if (apiKey) {
    headers['X-API-Key'] = apiKey;
  }

  return headers;
}
