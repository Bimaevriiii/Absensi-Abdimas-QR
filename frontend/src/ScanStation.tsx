import { useEffect, useRef, useState } from 'react';
import { Camera, CheckCircle2, LogIn, LogOut, Upload, XCircle, Info, ArrowRight, Maximize2, Minimize2 } from 'lucide-react';
import { apiJSON, idValidity } from './api';
import type { Person } from './attendance';

type Mode = 'masuk' | 'pulang';
type Item = { key: number; person?: Person; mode: Mode; time: string; ok: boolean; info: boolean; msg: string };

const COOLDOWN = 30000; // same card (per mode) is ignored this long after a read, so holding it up never duplicates
const clock = () => new Date().toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' });
const initials = (s: string) => s.split(' ').slice(0, 2).map(x => x[0]).join('').toUpperCase();

export default function ScanStation({ onRecorded }: { onRecorded: () => void }) {
  const [mode, setMode] = useState<Mode>('masuk');
  const [camera, setCamera] = useState(false);
  const [error, setError] = useState('');
  const [feed, setFeed] = useState<Item[]>([]);
  const [flash, setFlash] = useState<Item | null>(null);
  const [code, setCode] = useState('');
  const [full, setFull] = useState(false);
  const video = useRef<HTMLVideoElement>(null);
  const stream = useRef<MediaStream | null>(null);
  const gen = useRef(0);
  const starting = useRef(false);
  const seen = useRef(new Map<string, number>());
  const busy = useRef(false);
  const modeRef = useRef(mode);
  modeRef.current = mode;
  const fileInput = useRef<HTMLInputElement>(null);
  const key = useRef(0);
  const flashTimer = useRef(0);

  function toggleFull(on: boolean) {
    setFull(on);
    try { if (on) document.documentElement.requestFullscreen?.().catch(() => {}); else if (document.fullscreenElement) document.exitFullscreen().catch(() => {}); } catch {}
  }
  useEffect(() => {
    const sync = () => { if (!document.fullscreenElement) setFull(false); };
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') setFull(false); };
    document.addEventListener('fullscreenchange', sync); document.addEventListener('keydown', esc);
    return () => { document.removeEventListener('fullscreenchange', sync); document.removeEventListener('keydown', esc); if (document.fullscreenElement) document.exitFullscreen().catch(() => {}); };
  }, []);

  function stop() {
    gen.current++; starting.current = false;
    stream.current?.getTracks().forEach(t => t.stop()); stream.current = null; setCamera(false);
  }
  async function start() {
    if (starting.current || stream.current) return;
    starting.current = true; const g = ++gen.current; setError('');
    try {
      if (!window.isSecureContext) throw Error('Browser menganggap halaman ini tidak aman (sertifikat belum dipercaya), jadi kamera diblokir. Pakai HTTPS yang tepercaya, atau unggah QR.');
      if (!navigator.mediaDevices?.getUserMedia) throw Error('Kamera tidak tersedia di browser ini. Gunakan unggah QR atau pemindai USB.');
      const s = await navigator.mediaDevices.getUserMedia({ video: { facingMode: { ideal: 'environment' } }, audio: false });
      if (g !== gen.current) { s.getTracks().forEach(t => t.stop()); return; }
      stream.current = s; setCamera(true);
    } catch (e: any) {
      if (g === gen.current) setError(e.name === 'NotAllowedError' ? 'Izin kamera ditolak. Izinkan kamera di pengaturan browser, atau unggah foto QR.' : e.name === 'NotFoundError' ? 'Kamera tidak ditemukan. Gunakan unggah QR atau pemindai USB.' : e.message || 'Kamera tidak dapat dibuka.');
    } finally { if (g === gen.current) starting.current = false; }
  }
  useEffect(() => { start(); return () => { gen.current++; starting.current = false; stream.current?.getTracks().forEach(t => t.stop()); stream.current = null; clearTimeout(flashTimer.current); }; }, []);

  // Records one scan. Never throws; returns when the lock is released.
  async function record(raw: string) {
    const value = raw.trim();
    if (!value || busy.current) return;
    const now = Date.now();
    const m = modeRef.current;
    const k = m + ':' + value;
    if ((seen.current.get(k) ?? 0) > now) return;
    seen.current.set(k, now + COOLDOWN);
    busy.current = true;
    let item: Item;
    try {
      const d: any = await apiJSON('/api/hadir', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ action: 'scan', mode: m, token: value }) });
      item = { key: ++key.current, person: d.person, mode: m, time: clock(), ok: true, info: false, msg: d.message || 'Berhasil dicatat.' };
      onRecorded();
    } catch (e: any) {
      const dup = e.status === 409;
      item = { key: ++key.current, person: e.body?.person, mode: m, time: clock(), ok: false, info: dup, msg: e.message || 'Gagal memindai.' };
      // a rejected scan may be retried sooner than a successful one
      if (!dup) seen.current.set(k, now + 1500);
    } finally { busy.current = false; }
    setFeed(f => [item, ...f].slice(0, 30));
    setFlash(item); clearTimeout(flashTimer.current); flashTimer.current = window.setTimeout(() => setFlash(null), 2500);
  }

  useEffect(() => {
    if (!camera) return;
    const v = video.current, s = stream.current;
    if (!v || !s) return;
    let dead = false, id = 0, last = 0;
    v.srcObject = s; v.play().catch(() => {});
    const cvs = document.createElement('canvas'), ctx = cvs.getContext('2d', { willReadFrequently: true });
    import('jsqr').then(({ default: jsQR }) => {
      const loop = (ts: number) => {
        if (dead) return;
        if (ctx && v.readyState >= 2 && ts - last > 200) {
          last = ts; cvs.width = v.videoWidth; cvs.height = v.videoHeight; ctx.drawImage(v, 0, 0);
          const img = ctx.getImageData(0, 0, cvs.width, cvs.height);
          const qr = jsQR(img.data, img.width, img.height, { inversionAttempts: 'attemptBoth' });
          if (qr) record(qr.data);
        }
        id = requestAnimationFrame(loop);
      };
      if (!dead) id = requestAnimationFrame(loop);
    }).catch(() => setError('Pemindai tidak dapat dimuat. Gunakan unggah QR.'));
    return () => { dead = true; cancelAnimationFrame(id); };
  }, [camera]);

  async function upload(file?: File) {
    if (!file) return;
    try {
      const { default: jsQR } = await import('jsqr');
      const bmp = await createImageBitmap(file);
      const c = document.createElement('canvas'); c.width = bmp.width; c.height = bmp.height;
      const x = c.getContext('2d')!; x.drawImage(bmp, 0, 0); bmp.close();
      const d = x.getImageData(0, 0, c.width, c.height);
      const qr = jsQR(d.data, d.width, d.height, { inversionAttempts: 'attemptBoth' });
      if (!qr) throw Error('QR tidak terbaca. Gunakan foto yang lebih jelas.');
      await record(qr.data);
    } catch (e: any) {
      const item = { key: ++key.current, mode, time: clock(), ok: false, info: false, msg: e.message || 'QR tidak terbaca.' };
      setFeed(f => [item, ...f].slice(0, 30)); setFlash(item);
    }
    if (fileInput.current) fileInput.current.value = '';
  }

  const done = feed.filter(i => i.ok);
  return (
    <div className={'scan-station' + (full ? ' full' : '')}>
      <section className="station-camera">
        <div className={'station-video' + (camera ? ' running' : '')}>
          {camera ? <video ref={video} muted playsInline /> : (
            <div className="station-idle">
              <Camera size={46} strokeWidth={1.3} />
              <p>{error || 'Mengaktifkan kamera…'}</p>
              <button className="btn btn-white" onClick={start}><Camera size={17} />Aktifkan kamera</button>
            </div>
          )}
          {camera && <div className="camera-guide" />}
          <div className="station-modes" role="group" aria-label="Mode absensi">
            <button className={mode === 'masuk' ? 'active' : ''} aria-pressed={mode === 'masuk'} onClick={() => setMode('masuk')}><LogIn size={18} />Masuk</button>
            <button className={mode === 'pulang' ? 'active' : ''} aria-pressed={mode === 'pulang'} onClick={() => setMode('pulang')}><LogOut size={18} />Pulang</button>
          </div>
          <button className="station-full" onClick={() => toggleFull(!full)} aria-label={full ? 'Keluar dari tampilan penuh' : 'Tampilkan penuh'}>
            {full ? <><Minimize2 size={17} />Keluar</> : <><Maximize2 size={17} />Tampilkan penuh</>}
          </button>
          {flash && (
            <div className={'station-flash ' + (flash.ok ? 'ok' : flash.info ? 'info' : 'bad')} role="status">
              {flash.ok ? <CheckCircle2 size={26} /> : flash.info ? <Info size={26} /> : <XCircle size={26} />}
              <div><strong>{flash.person?.name || (flash.ok ? 'Berhasil' : flash.info ? 'Sudah tercatat' : 'Gagal')}</strong><span>{flash.msg}</span></div>
            </div>
          )}
        </div>
        <div className="station-tools">
          <button className="btn secondary" onClick={() => fileInput.current?.click()}><Upload size={16} />Unggah QR</button>
          <input ref={fileInput} type="file" accept="image/*" hidden onChange={e => upload(e.target.files?.[0])} />
          <form className="station-code" {...idValidity} onSubmit={e => { e.preventDefault(); record(code); setCode(''); }}>
            <input aria-label="Kode QR" placeholder="Pemindai USB / tempel kode kartu…" value={code} onChange={e => setCode(e.target.value)} required />
            <button className="btn">Catat<ArrowRight size={16} /></button>
          </form>
          {camera && <button className="btn secondary" onClick={stop}>Matikan kamera</button>}
        </div>
      </section>
      <aside className="station-card">
        <div className="station-card-head">
          <h2>Berhasil {mode === 'masuk' ? 'masuk' : 'pulang'}</h2>
          <span className="badge">{done.length} orang</span>
        </div>
        <p className="station-hint">{mode === 'masuk' ? 'Arahkan kartu ke kamera, absen tercatat otomatis.' : 'Mode pulang aktif. Kartu yang sudah masuk akan dicatat pulang.'}</p>
        <ol className="station-feed">
          {feed.length === 0 && <li className="station-empty">Belum ada pemindaian.</li>}
          {feed.map(i => (
            <li key={i.key} className={i.ok ? 'ok' : i.info ? 'info' : 'bad'}>
              <span className="avatar">{i.person ? initials(i.person.name) : '!'}</span>
              <div>
                <strong>{i.person?.name || (i.info ? 'Sudah tercatat' : 'Gagal')}</strong>
                <small>{i.person ? [i.person.group_name, i.mode === 'masuk' ? 'Masuk' : 'Pulang'].filter(Boolean).join(' · ') : i.msg}</small>
                {i.person && !i.ok && <small>{i.msg}</small>}
                {i.ok && <small>{i.msg}</small>}
              </div>
              <time>{i.time}</time>
            </li>
          ))}
        </ol>
      </aside>
    </div>
  );
}
