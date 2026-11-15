import { useEffect, useState, type FormEvent } from 'react';
import { ArrowRight, LockKeyhole, QrCode, ShieldCheck } from 'lucide-react';
import Dashboard from './Dashboard';
import { apiFetch } from './api';

export type Account = { id: string; email: string; name: string; role: 'pengelola' | 'operator' | 'petugas' };

export default function AuthGate() {
  const [user, setUser] = useState<Account | null>(null);
  const [checking, setChecking] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');

  useEffect(() => {
    let alive = true;
    apiFetch('/api/auth/me')
      .then(async response => {
        if (response.status === 401) return null;
        const body = await response.json();
        if (!response.ok) throw Error(body.error || 'Tidak dapat memeriksa sesi.');
        return body.user as Account;
      })
      .then(account => { if (alive) setUser(account); })
      .catch((cause: Error) => { if (alive) setError(cause.message || 'Server tidak dapat dihubungi.'); })
      .finally(() => { if (alive) setChecking(false); });
    return () => { alive = false; };
  }, []);

  useEffect(() => {
    const expired = () => { setUser(null); setError('Sesi berakhir. Silakan masuk kembali.'); };
    window.addEventListener('hadir-session-expired', expired);
    return () => window.removeEventListener('hadir-session-expired', expired);
  }, []);

  async function login(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      const response = await apiFetch('/api/auth/login', {
        method: 'POST', credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password }),
      });
      const body = await response.json();
      if (!response.ok) throw Error(body.error || 'Gagal masuk.');
      setPassword('');
      setUser(body.user as Account);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Server tidak dapat dihubungi.');
    } finally {
      setBusy(false);
    }
  }

  async function logout() {
    setBusy(true);
    try {
      const response = await apiFetch('/api/auth/logout', {
        method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: '{}' });
      if (!response.ok) throw Error('Gagal keluar. Coba lagi.');
      setUser(null);
      setError('');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Server tidak dapat dihubungi.');
    } finally {
      setBusy(false);
    }
  }

  if (checking) return <div className="auth-screen"><div className="auth-loading">Memeriksa sesi…</div></div>;
  if (user) return <Dashboard user={user} logout={logout} logoutBusy={busy} authError={error} />;

  return <div className="auth-screen"><div className="auth-card">
    <div className="auth-brand"><span className="brand-icon"><QrCode size={25}/></span>hadir<span className="brand-dot">.</span></div>
    <div className="auth-eyebrow"><ShieldCheck size={16}/> RUANG KERJA SEKOLAH</div>
    <h1>Selamat datang kembali.</h1>
    <p>Masuk untuk mengelola dan mencatat kehadiran murid serta guru.</p>
    <form onSubmit={login}>
      <label>Email akun<input autoComplete="username" type="email" value={email} onChange={event => setEmail(event.target.value)} required /></label>
      <label>Kata sandi<input autoComplete="current-password" type="password" value={password} onChange={event => setPassword(event.target.value)} required /></label>
      {error && <div className="auth-error" role="alert">{error}</div>}
      <button className="btn full" disabled={busy}><LockKeyhole size={17}/>{busy ? 'Memeriksa…' : 'Masuk ke Hadir'}<ArrowRight size={17}/></button>
    </form>
    <small>Gunakan akun yang diberikan pengelola sekolah.</small>
  </div></div>;
}
