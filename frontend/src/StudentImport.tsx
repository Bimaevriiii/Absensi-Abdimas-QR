import { useState, type FormEvent } from 'react';
import { Download, Upload, X } from 'lucide-react';
import { apiJSON, useDialog } from './api';

const importPath = '/api/students/import';
const xlsxType = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet';
const maxUpload = 1024 * 1024;

type Issue = { row: number; column: string; reason: string };
type ImportResult = { new: number; added: number; skipped: Issue[]; errors: Issue[] };
type Mode = 'preview' | 'commit';

function ImportIssues({ title, issues }: { title: string; issues: Issue[] }) {
  if (!issues.length) return null;
  return <section className="student-import-issues" aria-label={title}>
    <h3>{title} ({issues.length})</h3>
    <ul>{issues.map(issue => <li key={issue.row + '-' + issue.column}>
      <strong>Baris {issue.row} · {issue.column}</strong>
      <span>{issue.reason}</span>
    </li>)}</ul>
  </section>;
}

function ImportDialog({ onClose, onImported }: { onClose: () => void; onImported: () => Promise<void> }) {
  const [file, setFile] = useState<File | null>(null);
  const [result, setResult] = useState<ImportResult | null>(null);
  const [busy, setBusy] = useState<Mode | null>(null);
  const [error, setError] = useState('');
  const [completed, setCompleted] = useState(false);

  const close = () => { if (!busy) onClose(); };
  const dialog = useDialog<HTMLFormElement>(true, close);
  const canSave = result && result.new > 0 && !result.errors.length && !completed;

  async function send(mode: Mode) {
    if (!file || busy || (mode === 'commit' && !canSave)) return;
    setError('');
    if (mode === 'preview') setResult(null);
    if (!/\.xlsx$/i.test(file.name)) {
      setError('Pilih file dengan ekstensi .xlsx.');
      return;
    }
    if (file.size > maxUpload) {
      setError('Ukuran file maksimal 1 MiB.');
      return;
    }

    setBusy(mode);
    try {
      const response = await apiJSON<ImportResult>(importPath + '?mode=' + mode, {
        method: 'POST',
        headers: { 'Content-Type': xlsxType },
        body: file,
      }, 'Impor gagal. Periksa file dan coba lagi.');
      setResult(response);
      if (mode === 'commit') {
        setCompleted(true);
        await onImported();
      }
    } catch (cause) {
      const failure = cause as Error & { status?: number; body?: ImportResult };
      if (failure.status === 422 && Array.isArray(failure.body?.errors)) setResult(failure.body);
      setError(failure.status === 413 ? 'Ukuran file maksimal 1 MiB.' : failure.message);
    } finally {
      setBusy(null);
    }
  }

  function preview(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void send('preview');
  }

  return <div className="modal-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) close(); }}>
    <form className="modal student-import-form" ref={dialog} role="dialog" aria-modal="true" aria-label="Impor Excel murid" onSubmit={preview}>
      <div className="modal-head">
        <div><h2>Impor Excel murid</h2><p>Isi sheet Murid pada contoh Excel. Gunakan format Teks untuk NIS Lokal, NISN, dan NIK.</p></div>
        <button type="button" className="icon-btn" aria-label="Tutup impor" onClick={close} disabled={!!busy}><X size={20}/></button>
      </div>
      <label>File Excel (.xlsx, maksimal 1 MiB)
        <input type="file" accept=".xlsx" disabled={!!busy} onChange={event => {
          setFile(event.target.files?.[0] ?? null);
          setResult(null);
          setError('');
          setCompleted(false);
        }}/>
      </label>
      {error && <p className="account-error" role="alert">{error}</p>}
      {result && <div className="student-import-result">
        <p role="status">
          <strong>{completed ? result.added + ' murid ditambahkan' : result.new + ' murid baru'}</strong>
          <span>{result.skipped.length} baris dilewati</span>
        </p>
        <ImportIssues title="Perlu diperbaiki" issues={result.errors}/>
        <ImportIssues title="Baris yang dilewati" issues={result.skipped}/>
      </div>}
      <div className="modal-actions">
        {completed
          ? <button type="button" className="btn" onClick={close} disabled={!!busy}>Selesai</button>
          : <>
            <button type="submit" className="btn secondary" disabled={!file || !!busy}>{busy === 'preview' ? 'Memeriksa…' : 'Periksa file'}</button>
            <button type="button" className="btn" disabled={!canSave || !!busy} onClick={() => void send('commit')}>{busy === 'commit' ? 'Menyimpan…' : 'Simpan murid baru'}</button>
          </>}
      </div>
    </form>
  </div>;
}

export default function StudentImport({ disabled, onImported }: { disabled: boolean; onImported: () => Promise<void> }) {
  const [open, setOpen] = useState(false);
  return <div className="student-import">
    <a className="btn secondary" href={importPath + '/template'} download="contoh-impor-murid.xlsx"><Download size={16}/>Unduh contoh Excel</a>
    <button className="btn secondary" type="button" onClick={() => setOpen(true)} disabled={disabled}><Upload size={16}/>Impor Excel</button>
    {open && <ImportDialog onClose={() => setOpen(false)} onImported={onImported}/>}
  </div>;
}
