import { useState, type FormEvent } from 'react';
import { ClipboardCheck, Search } from 'lucide-react';
import { apiFetch } from './api';
import type { RecordItem } from './attendance';

type Identity = { id: string; name: string; number: string; role: string; group_name: string };

export default function GateManualStatus({ today, onSaved }: { today: string; onSaved: () => Promise<void> }) {
  const [number, setNumber] = useState('');
  const [found, setFound] = useState<{ person: Identity; record: RecordItem | null } | null>(null);
  const [status, setStatus] = useState('Izin');
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');

  async function lookup(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError('');
    setMessage('');
    setFound(null);
    try {
      const response = await apiFetch('/api/hadir', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ action: 'lookupNumber', number }),
      });
      const body = await response.json();
      if (!response.ok) throw Error(body.error || 'Nomor induk tidak ditemukan.');
      setFound(body);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Pemeriksaan gagal.');
    } finally {
      setBusy(false);
    }
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!found || found.record) return;
    setBusy(true);
    setError('');
    setMessage('');
    try {
      const response = await apiFetch('/api/hadir', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: 'manual', person_id: found.person.id, day: today, status, reason }),
      });
      const body = await response.json();
      if (!response.ok) throw Error(body.error || 'Status gagal disimpan.');
      setFound({ person: found.person, record: body.record });
      setMessage(body.message);
      setReason('');
      await onSaved();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Status gagal disimpan.');
    } finally {
      setBusy(false);
    }
  }

  return <section className="panel manual-panel gate-manual">
    <div className="section-head"><div><h2>Catat izin, sakit, atau alpa</h2><p>Input manual untuk hari ini. Periksa nomor induk dan identitas terlebih dahulu.</p></div><ClipboardCheck size={21}/></div>
    <form className="manual-form" onSubmit={lookup}>
      <label>Nomor induk siswa/guru<input value={number} onChange={event => { setNumber(event.target.value); setFound(null); }} maxLength={40} placeholder="NIS atau NIP tepat" required /></label>
      <button className="btn secondary" disabled={busy}><Search size={16}/>Cari identitas</button>
    </form>
    {error && <p className="account-error" role="alert">{error}</p>}
    {message && <p className="account-success" role="status">{message}</p>}
    {found && <div className="manual-found">
      <strong>{found.person.name}</strong><span>{found.person.number} · {found.person.group_name} · {found.person.role}</span>
      {found.record ? <p>Catatan hari ini sudah ada: <strong>{found.record.status}</strong>. Koreksi hanya dapat dilakukan pengelola melalui rekap.</p> :
        <form onSubmit={save} className="manual-status-form">
          <label>Status<select value={status} onChange={event => setStatus(event.target.value)}><option>Izin</option><option>Sakit</option><option>Alpa</option></select></label>
          <label>Alasan / keterangan<textarea value={reason} onChange={event => setReason(event.target.value)} minLength={5} maxLength={500} required placeholder="Contoh: Surat izin diterima" /></label>
          <button className="btn" disabled={busy}><ClipboardCheck size={16}/>{busy ? 'Menyimpan…' : 'Catat status'}</button>
        </form>}
    </div>}
  </section>;
}
