import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { Check, KeyRound, Plus, ShieldCheck, X } from 'lucide-react';
import { apiJSON, idValidity, useDialog } from './api';
import { roleLabel } from './attendance';

type ManagedUser = { id: string; email: string; name: string; role: string; active: boolean };

export default function UserManagement({ currentUserId }: { currentUserId: string }) {
  const [users, setUsers] = useState<ManagedUser[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [resetUser, setResetUser] = useState<ManagedUser | null>(null);

  const dialog = useDialog<HTMLFormElement>(Boolean(resetUser), () => setResetUser(null));

  const load = useCallback(async () => {
    try {
      const body = await apiJSON('/api/users', undefined, 'Gagal memuat akun.');
      setUsers(body.users);
      setError('');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Server tidak dapat dihubungi.');
    }
  }, []);

  useEffect(() => { void load(); }, [load]);

  async function action(body: Record<string, string>) {
    setBusy(true);
    setError('');
    setMessage('');
    try {
      const result = await apiJSON('/api/users', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
      });
      setMessage(result.message || 'Akun berhasil dibuat.');
      await load();
      return true;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Server tidak dapat dihubungi.');
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const fields = new FormData(form);
    const done = await action({
      action: 'create', name: String(fields.get('name') || ''), email: String(fields.get('email') || ''),
      role: String(fields.get('role') || ''), password: String(fields.get('password') || ''),
    });
    if (done) form.reset();
  }

  async function reset(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!resetUser) return;
    const fields = new FormData(event.currentTarget);
    const done = await action({ action: 'resetPassword', id: resetUser.id, password: String(fields.get('password') || '') });
    if (done) setResetUser(null);
  }

  async function deactivate(user: ManagedUser) {
    if (!window.confirm(`Nonaktifkan akun ${user.name}? Semua sesi akun ini akan berakhir.`)) return;
    await action({ action: 'deactivate', id: user.id });
  }

  return <section className="panel account-panel">
    <div className="section-head"><div><h2>Akun petugas</h2><p>Pengelola memberi akses sesuai tugas masing-masing.</p></div><ShieldCheck size={22}/></div>
    {error && <p className="account-error" role="alert">{error}</p>}
    {message && <p className="account-success" role="status">{message}</p>}
    <div className="table-scroll"><table><thead><tr><th>Nama</th><th>Email</th><th>Peran</th><th>Status</th><th>Tindakan</th></tr></thead><tbody>
      {users.map(user => <tr key={user.id}>
        <td><strong>{user.name}</strong></td><td>{user.email}</td><td>{user.active && user.id !== currentUserId ? <select aria-label={'Peran '+user.name} value={user.role} disabled={busy} onChange={event => void action({ action: 'setRole', id: user.id, role: event.target.value })}><option value="petugas">Petugas gerbang</option><option value="operator">Operator</option><option value="pengelola">Pengelola</option></select> : roleLabel(user.role)}</td>
        <td><span className={'badge '+(user.active?'':'neutral')}>{user.active?'Aktif':'Nonaktif'}</span></td>
        <td><div className="account-actions">
          {user.active && <button className="text-btn" disabled={busy} onClick={() => setResetUser(user)}><KeyRound size={15}/>Reset sandi</button>}
          {!user.active && <button className="text-btn" disabled={busy} onClick={() => void action({ action: 'activate', id: user.id })}>Aktifkan</button>}
          {user.active && user.id !== currentUserId && <button className="text-btn danger" disabled={busy} onClick={() => deactivate(user)}>Nonaktifkan</button>}
        </div></td>
      </tr>)}
    </tbody></table></div>
    <form className="account-form" onSubmit={create} {...idValidity}>
      <h3>Tambah akun</h3>
      <div className="account-fields">
        <label>Nama<input name="name" maxLength={100} required /></label>
        <label>Email<input name="email" type="email" autoComplete="off" required /></label>
        <label>Peran<select name="role" defaultValue="petugas"><option value="petugas">Petugas gerbang</option><option value="operator">Operator</option><option value="pengelola">Pengelola</option></select></label>
        <label>Kata sandi awal (minimal 12 karakter)<input name="password" type="password" minLength={12} autoComplete="new-password" required /></label>
      </div>
      <button className="btn" disabled={busy}><Plus size={16}/>{busy?'Menyimpan…':'Tambah akun'}</button>
    </form>
    {resetUser && <div className="modal-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) setResetUser(null); }}>
      <form className="modal" ref={dialog} role="dialog" aria-modal="true" aria-label="Reset kata sandi" onSubmit={reset} {...idValidity}>
        <div className="modal-head"><div><h2>Reset kata sandi</h2><p>{resetUser.name} · {resetUser.email}</p></div><button className="icon-btn" type="button" aria-label="Tutup" onClick={() => setResetUser(null)}><X size={20}/></button></div>
        <label>Kata sandi baru (minimal 12 karakter)<input name="password" type="password" minLength={12} autoComplete="new-password" required /></label>
        <p className="field-note">Minimal 12 karakter. Semua sesi lama akun ini akan berakhir.</p>
        <div className="modal-actions"><button className="btn secondary" type="button" onClick={() => setResetUser(null)}>Batal</button><button className="btn" disabled={busy}><Check size={16}/>Simpan sandi</button></div>
      </form>
    </div>}
  </section>;
}
