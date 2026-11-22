import { useState, type FormEvent } from 'react';
import { ClipboardCheck, Search } from 'lucide-react';
import { apiJSON, idValidity } from './api';
import { roleLabel, type RecordItem } from './attendance';

type Identity = { id: string; name: string; number: string; role: string; group_name: string };

export default function GateManualStatus({ today, onSaved }: { today: string; onSaved: () => Promise<void> }) {
  const [number, setNumber] = useState('');
  const [numberType, setNumberType] = useState('');
  const [chooseNumberType, setChooseNumberType] = useState(false);
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
      const body = await apiJSON('/api/hadir', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ action: 'lookupNumber', number, number_type: numberType }),
      }, 'Nomor induk tidak ditemukan.');
      setFound(body);
    } catch (cause) {
      if ((cause as { body?: { code?: string } })?.body?.code === 'ambiguous_number') setChooseNumberType(true);
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
      const body = await apiJSON('/api/hadir', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: 'manual', person_id: found.person.id, day: today, status, reason }),
      }, 'Status gagal disimpan.');
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
    <form className="manual-form" onSubmit={lookup} {...idValidity}>
      <label>NIS Lokal, NISN, atau NIP<input value={number} onChange={event => {
        setNumber(event.target.value);
        setFound(null);
        setChooseNumberType(false);
        setNumberType('');
        setError('');
        setMessage('');
      }} maxLength={40} disabled={busy} required /></label>
      {chooseNumberType && <label>Jenis nomor<select value={numberType} onChange={event => {
        setNumberType(event.target.value);
        setFound(null);
        setMessage('');
      }} disabled={busy} required>
        <option value="">Pilih jenis nomor</option><option value="number">NIS Lokal / NIP</option><option value="nisn">NISN</option>
      </select></label>}
      <button className="btn secondary" disabled={busy}><Search size={16}/>Cari identitas</button>
    </form>
    {error && <p className="account-error" role="alert">{error}</p>}
    {message && <p className="account-success" role="status">{message}</p>}
    {found && <div className="manual-found">
      <strong>{found.person.name}</strong><span>{found.person.number} · {found.person.group_name} · {roleLabel(found.person.role)}</span>
      {found.record ? <p>Catatan hari ini sudah ada: <strong>{found.record.status}</strong>. Koreksi hanya dapat dilakukan pengelola melalui rekap.</p> :
        <form onSubmit={save} className="manual-status-form" {...idValidity}>
          <label>Status<select value={status} onChange={event => setStatus(event.target.value)}><option>Izin</option><option>Sakit</option><option>Alpa</option></select></label>
          <label>Alasan / keterangan<textarea value={reason} onChange={event => setReason(event.target.value)} minLength={5} maxLength={500} required placeholder="Contoh: Surat izin diterima" /></label>
          <button className="btn" disabled={busy}><ClipboardCheck size={16}/>{busy ? 'Menyimpan…' : 'Catat status'}</button>
        </form>}
    </div>}
  </section>;
}
