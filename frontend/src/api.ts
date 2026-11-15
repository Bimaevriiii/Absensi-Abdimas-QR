export async function apiFetch(input: string, init?: RequestInit): Promise<Response> {
  let response: Response;
  try {
    response = await fetch(input, { credentials: 'same-origin', cache: 'no-store', ...init });
  } catch (cause) {
    if (cause instanceof TypeError) throw Error('Tidak dapat terhubung ke server. Pastikan perangkat online.');
    throw cause;
  }
  if (response.status === 401 && !input.startsWith('/api/auth/')) window.dispatchEvent(new Event('hadir-session-expired'));
  return response;
}
