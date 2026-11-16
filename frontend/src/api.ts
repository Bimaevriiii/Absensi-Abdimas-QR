import { useEffect, useRef, type FormEvent } from 'react';

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

// Fetch + parse JSON; non-JSON (e.g. nginx HTML 502) never leaks parser errors. Thrown Error has `.status`.
export async function apiJSON<T = any>(input: string, init?: RequestInit, fallback = 'Permintaan gagal.'): Promise<T> {
  const response = await apiFetch(input, init);
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    const message = typeof body?.error === 'string' && body.error ? body.error : response.status >= 500 ? 'Server sedang tidak tersedia. Coba lagi sebentar.' : response.status === 429 ? 'Terlalu banyak percobaan. Coba lagi nanti.' : fallback;
    throw Object.assign(Error(message), { status: response.status, body });
  }
  return body as T;
}

// Spread on <form> for Indonesian native validation messages.
export const idValidity = {
  onInvalidCapture: (event: FormEvent<HTMLFormElement>) => {
    const t = event.target as HTMLInputElement;
    t.setCustomValidity(t.validity.valueMissing ? 'Isi kolom ini.' : t.validity.tooShort ? `Minimal ${t.minLength} karakter.` : t.validity.typeMismatch ? 'Format tidak valid.' : '');
  },
  onInputCapture: (event: FormEvent<HTMLFormElement>) => (event.target as HTMLInputElement).setCustomValidity(''),
};

// Modal a11y: Escape closes, focus moves in on open and returns on close. ponytail: no Tab trap.
export function useDialog<T extends HTMLElement>(open: boolean, onClose: () => void) {
  const ref = useRef<T>(null);
  const close = useRef(onClose);
  close.current = onClose;
  useEffect(() => {
    if (!open) return;
    const before = document.activeElement as HTMLElement | null;
    ref.current?.querySelector<HTMLElement>('input,select,textarea,button')?.focus();
    const key = (e: KeyboardEvent) => { if (e.key === 'Escape') close.current(); };
    document.addEventListener('keydown', key);
    return () => { document.removeEventListener('keydown', key); before?.focus?.(); };
  }, [open]);
  return ref;
}
