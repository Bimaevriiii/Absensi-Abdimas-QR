import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from 'react';
import { Check, ClipboardCheck, History } from 'lucide-react';
import { apiJSON, idValidity } from './api';
import { isoToSchoolInput, schoolInputToISO, schoolTime, type Person, type RecordItem, type SchoolSettings } from './attendance';

type AuditEntry = {
  id: string; attendance_id: string; person_id: string; actor_name: string; action: string;
  before: RecordItem | null; after: RecordItem; reason: string; created_at: string;
};

function snapshot(record: RecordItem | null, timezone: string): string {
  if (!record) return 'Belum ada catatan';
  return `${record.status} · Masuk ${schoolTime(record.arrived, timezone)} · Pulang ${schoolTime(record.departed, timezone)}${record.note ? ' · ' + record.note : ''}`;
}

export default function AttendanceManagement({ people, records, settings, today, role, from, to, onSaved }: {
  people: Person[]; records: RecordItem[]; settings: SchoolSettings; today: string; role: string;
  from: string; to: string; onSaved: () => Promise<void>;
}) {
  const admin = role === 'pengelola';
  const [personID, setPersonID] = useState('');
  const [day, setDay] = useState(today);
  const [manualStatus, setManualStatus] = useState('Izin');
  const [manualReason, setManualReason] = useState('');
  const [manualNote, setManualNote] = useState('');
  const [manualArrived, setManualArrived] = useState(`${today}T07:00`);
  const [manualDeparted, setManualDeparted] = useState('');
  const [selectedID, setSelectedID] = useState('');
  const [correctStatus, setCorrectStatus] = useState('Hadir');
  const [correctArrived, setCorrectArrived] = useState('');
  const [correctDeparted, setCorrectDeparted] = useState('');
  const [correctNote, setCorrectNote] = useState('');
  const [correctReason, setCorrectReason] = useState('');
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');

  const dirty = useRef(false);
  const lastID = useRef('');
  const banner = useRef<HTMLDivElement>(null);
  const edit = (set: (v: string) => void) => (event: { target: { value: string } }) => { dirty.current = true; set(event.target.value); };
  const visibleRecords = useMemo(() => records.filter(record => record.day >= from && record.day <= to), [records, from, to]);
  const selected = records.find(record => record.id === selectedID);
  const personName = (id: string) => people.find(person => person.id === id)?.name || 'Peserta tidak tersedia';

  useEffect(() => {
    if (!selected || (lastID.current === selected.id && dirty.current)) return; // keep half-typed edits across polls
    lastID.current = selected.id;
    dirty.current = false;
    setCorrectStatus(selected.arrived ? 'Hadir' : selected.status);
    setCorrectArrived(isoToSchoolInput(selected.arrived, settings.timezone));
    setCorrectDeparted(isoToSchoolInput(selected.departed, settings.timezone));
    setCorrectNote(selected.note);
    setCorrectReason('');
  }, [selected?.id, selected?.arrived, selected?.departed, selected?.status, selected?.note, settings.timezone]);

  const loadAudit = useCallback(async () => {
    if (!admin) return;
    try {
      const query = selectedID ? `?attendance_id=${encodeURIComponent(selectedID)}` : '';
      const body = await apiJSON('/api/audit' + query, undefined, 'Riwayat tidak dapat dimuat.');
      setAudit(body.audit);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Riwayat tidak dapat dimuat.');
    }
  }, [admin, selectedID]);

  useEffect(() => { if (message || error) banner.current?.scrollIntoView({ block: 'nearest', behavior: 'smooth' }); }, [message, error]);
  useEffect(() => { void loadAudit(); }, [loadAudit]);

  async function send(body: Record<string, string>) {
    setBusy(true);
    setError('');
    setMessage('');
    try {
      const result = await apiJSON('/api/hadir', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }, 'Catatan gagal disimpan.');
      setMessage(result.message);
      await onSaved();
      await loadAudit();
      return true;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Catatan gagal disimpan.');
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function addManual(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      const body: Record<string, string> = { action: 'manual', person_id: personID, day, status: manualStatus, reason: manualReason, note: manualNote };
      if (manualStatus === 'Hadir') {
        body.arrived = schoolInputToISO(manualArrived, settings.timezone);
        if (manualDeparted) body.departed = schoolInputToISO(manualDeparted, settings.timezone);
      }
      if (await send(body)) { setManualReason(''); setManualNote(''); setPersonID(''); }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Waktu tidak valid.');
    }
  }

  async function correct(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selected) return;
    try {
      const body: Record<string, string> = { action: 'correct', id: selected.id, status: correctStatus, reason: correctReason, note: correctNote };
      if (correctStatus === 'Hadir') {
        body.arrived = schoolInputToISO(correctArrived, settings.timezone);
        if (correctDeparted) body.departed = schoolInputToISO(correctDeparted, settings.timezone);
      }
      dirty.current = false;
      if (await send(body)) setCorrectReason('');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Waktu tidak valid.');
    }
  }

  return <section className="panel manual-panel report-management">
    <div className="section-head"><div><h2>Catatan manual dan koreksi</h2><p>Izin, sakit, dan alpa diinput petugas. Koreksi catatan dilakukan pengelola dengan alasan.</p></div><ClipboardCheck size={22}/></div>
    <div ref={banner}>
      {error && <p className="account-error" role="alert">{error}</p>}
      {message && <p className="account-success" role="status">{message}</p>}
    </div>
    <div className="management-grid">
      <form className="management-form" onSubmit={addManual} {...idValidity}>
        <h3>Tambah catatan manual</h3>
        <label>Peserta<select value={personID} onChange={event => setPersonID(event.target.value)} required><option value="">Pilih murid/guru</option>{people.filter(person => person.active).map(person => <option key={person.id} value={person.id}>{person.name} · {person.number}</option>)}</select></label>
        <div className="form-two"><label>Tanggal sekolah<input type="date" min="2000-01-01" max={today} value={day} onChange={event => { const nextDay = event.target.value; setDay(nextDay); setManualArrived(`${nextDay}T07:00`); setManualDeparted(''); }} required /></label><label>Status<select value={manualStatus} onChange={event => setManualStatus(event.target.value)}><option>Izin</option><option>Sakit</option><option>Alpa</option>{admin && <option value="Hadir">Hadir terlewat</option>}</select></label></div>
        {manualStatus === 'Hadir' && <div className="form-two"><label>Masuk ({settings.timezone})<input type="datetime-local" value={manualArrived} onChange={event => setManualArrived(event.target.value)} required /></label><label>Pulang ({settings.timezone})<input type="datetime-local" value={manualDeparted} onChange={event => setManualDeparted(event.target.value)} /></label></div>}
        <label>Alasan pencatatan<textarea value={manualReason} onChange={event => setManualReason(event.target.value)} minLength={5} maxLength={500} required placeholder="Contoh: Surat izin diterima" /></label>
        <label>Keterangan catatan (opsional)<textarea value={manualNote} onChange={event => setManualNote(event.target.value)} maxLength={500} placeholder="Ditampilkan pada rekap" /></label>
        <button className="btn" disabled={busy}><Check size={16}/>Simpan catatan</button>
      </form>
      {admin && <div className="management-form">
        <h3>Koreksi catatan</h3>
        <label>Catatan pada periode rekap<select value={selectedID} onChange={event => setSelectedID(event.target.value)}><option value="">Pilih catatan</option>{visibleRecords.map(record => <option key={record.id} value={record.id}>{record.day} · {personName(record.person_id)} · {record.status}</option>)}</select></label>
        {selected ? <form className="correction-form" onSubmit={correct} {...idValidity}>
          <div className="form-two"><label>Status baru<select value={correctStatus} onChange={edit(setCorrectStatus)}><option value="Hadir">{selected.arrived ? `Hadir (saat ini ${selected.status})` : 'Hadir'}</option><option>Izin</option><option>Sakit</option><option>Alpa</option></select></label><label>Tanggal sekolah<input value={selected.day} disabled /></label></div>
          {correctStatus === 'Hadir' && <div className="form-two"><label>Masuk ({settings.timezone})<input type="datetime-local" value={correctArrived} onChange={edit(setCorrectArrived)} required /></label><label>Pulang ({settings.timezone})<input type="datetime-local" value={correctDeparted} onChange={edit(setCorrectDeparted)} /></label></div>}
          <label>Keterangan catatan<textarea value={correctNote} onChange={edit(setCorrectNote)} maxLength={500} /></label>
          <label>Alasan koreksi<textarea value={correctReason} onChange={edit(setCorrectReason)} minLength={5} maxLength={500} required placeholder="Jelaskan dasar perubahan" /></label>
          <button className="btn" disabled={busy}><Check size={16}/>Simpan koreksi</button>
        </form> : <p className="field-note">Pilih catatan yang sudah ada dari periode rekap di atas. Tanggal catatan tetap; waktu dan status dapat dikoreksi.</p>}
      </div>}
    </div>
    {admin && <div className="audit-panel"><div className="section-head"><div><h2>Riwayat perubahan</h2><p>{selectedID ? 'Perubahan untuk catatan terpilih' : '100 perubahan terbaru'}</p></div><History size={20}/></div>
      {audit.length ? <div className="audit-list">{audit.map(entry => <div className="audit-item" key={entry.id}>
        <strong>{personName(entry.person_id)} · {entry.action === 'manual_create' ? 'Input manual' : 'Koreksi'}</strong>
        <small>{entry.actor_name} · {new Intl.DateTimeFormat('id-ID', { timeZone: settings.timezone, dateStyle: 'medium', timeStyle: 'short' }).format(new Date(entry.created_at))}</small>
        <p>Alasan: {entry.reason}</p>
        <div><span>Sebelum: {snapshot(entry.before, settings.timezone)}</span><span>Sesudah: {snapshot(entry.after, settings.timezone)}</span></div>
      </div>)}</div> : <p className="field-note audit-empty">Belum ada catatan perubahan.</p>}
    </div>}
  </section>;
}
