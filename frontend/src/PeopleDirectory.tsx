import { useState, type FormEvent } from 'react';
import { Check, QrCode, X } from 'lucide-react';
import { idValidity, useDialog } from './api';
import type { Person } from './attendance';

export const initials = (name: string) => name.split(' ').slice(0, 2).map(part => part[0]).join('').toUpperCase();

export function Avatar({ p }: { p: Person }) {
  return <span className={'avatar ' + (p.role === 'guru' ? 'teacher' : '')}>{initials(p.name)}</span>;
}

export type StudentProfile = {
  nisn: string;
  gender: string;
  class_parallel: string;
  birth_place: string;
  birth_date: string;
  nik: string;
  home_address: string;
};

export type StudentProfiles = Record<string, StudentProfile>;

type PersonAction = {
  action: 'person';
  id?: string;
  name: string;
  number: string;
  role: string;
  group: string;
  student?: StudentProfile;
};

const birthDateFormat = new Intl.DateTimeFormat('id-ID', {
  day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC',
});

function birthLabel(profile?: StudentProfile) {
  const date = profile?.birth_date ? birthDateFormat.format(new Date(`${profile.birth_date}T00:00:00Z`)) : '';
  return [profile?.birth_place, date].filter(Boolean).join(', ');
}

export function PeopleTable({ people, showProfiles, profiles, busy, onCard, onEdit, onToggle }: {
  people: Person[];
  showProfiles: boolean;
  profiles: StudentProfiles | null;
  busy: boolean;
  onCard: (person: Person) => void;
  onEdit: (person: Person) => void;
  onToggle: (person: Person) => void;
}) {
  const headings = showProfiles
    ? ['NIS Lokal', 'NISN', 'Nama Siswa', 'Jenis Kelamin', 'Kelas', 'Kelas Paralel', 'Tempat/Tanggal Lahir', 'NIK Siswa', 'Alamat Rumah']
    : ['Nama lengkap', 'Nomor induk', 'Jenis', 'Kelas / bagian'];
  return <div className="table-scroll" tabIndex={showProfiles ? 0 : undefined} role={showProfiles ? 'region' : undefined} aria-label={showProfiles ? 'Tabel biodata murid' : undefined}>
    <table className={showProfiles ? 'student-table' : undefined}>
      <thead><tr>{[...headings, 'Status', 'Tindakan'].map(label => <th key={label} scope="col">{label}</th>)}</tr></thead>
      <tbody>{people.map(person => {
        const profile = profiles?.[person.id];
        const number = <td className="mono">{person.number}</td>;
        const name = <td><div className="person-cell">
          <Avatar p={person}/>
          <div><strong>{person.name}</strong>{!!person.demo && <small>Data contoh</small>}</div>
        </div></td>;
        return <tr key={person.id}>
          {showProfiles ? <>
            {number}<td className="mono">{profile?.nisn || '—'}</td>{name}
            <td>{profile?.gender || '—'}</td><td>{person.group_name}</td><td>{profile?.class_parallel || '—'}</td>
            <td>{birthLabel(profile) || '—'}</td><td className="mono">{profile?.nik || '—'}</td>
            <td className="student-address">{profile?.home_address || '—'}</td>
          </> : <>
            {name}{number}<td>{person.role === 'guru' ? 'Guru' : 'Murid'}</td><td>{person.group_name}</td>
          </>}
          <td><span className={'badge ' + (!person.active ? 'neutral' : '')}>{person.active ? 'Aktif' : 'Nonaktif'}</span></td>
          <td><div className="row-actions">
            <button title="Lihat kartu QR" aria-label={'Kartu QR ' + person.name} onClick={() => onCard(person)}><QrCode size={18}/></button>
            <button className="link" disabled={busy || (person.role === 'murid' && !profile)} onClick={() => onEdit(person)}>Edit</button>
            <button className="link muted" disabled={busy} onClick={() => onToggle(person)}>{person.active ? 'Nonaktifkan' : 'Aktifkan'}</button>
          </div></td>
        </tr>;
      })}</tbody>
    </table>
  </div>;
}

export function PersonForm({ person, profile, busy, onClose, onSave }: {
  person: Partial<Person>;
  profile?: StudentProfile;
  busy: boolean;
  onClose: () => void;
  onSave: (action: PersonAction) => Promise<void>;
}) {
  const [role, setRole] = useState(person.role || 'murid');
  const dialog = useDialog<HTMLFormElement>(true, onClose);
  const isStudent = role === 'murid';
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const fields = new FormData(event.currentTarget);
    const value = (name: string) => String(fields.get(name) ?? '');
    const action: PersonAction = {
      action: 'person', id: person.id, role, name: value('name'), number: value('number'), group: value('group'),
    };
    if (isStudent) {
      action.student = {
        nisn: value('nisn'), gender: value('gender'), class_parallel: value('class_parallel'),
        birth_place: value('birth_place'), birth_date: value('birth_date'), nik: value('nik'), home_address: value('home_address'),
      };
    }
    await onSave(action);
  }
  return <div className="modal-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) onClose(); }}>
    <form className={'modal' + (isStudent ? ' student-form' : '')} ref={dialog} role="dialog" aria-modal="true" aria-label={person.id ? 'Edit data' : 'Tambah warga sekolah'} {...idValidity} onSubmit={save}>
      <div className="modal-head">
        <div><h2>{person.id ? 'Edit data' : 'Tambah warga sekolah'}</h2><p>QR unik dibuat saat data pertama kali disimpan.</p></div>
        <button type="button" className="icon-btn" aria-label="Tutup formulir" onClick={onClose}><X size={20}/></button>
      </div>
      <label>Jenis<select name="role" value={role} onChange={event => setRole(event.target.value)}><option value="murid">Murid</option><option value="guru">Guru</option></select></label>
      <label>{isStudent ? 'Nama Siswa' : 'Nama lengkap'}<input name="name" defaultValue={person.name} placeholder="Contoh: Alya Putri" maxLength={100} required/></label>
      <label>{isStudent ? 'NIS Lokal' : 'Nomor induk (NIS / NIP)'}<input name="number" defaultValue={person.number} placeholder="Nomor unik setiap orang" maxLength={40} required/></label>
      <label>{isStudent ? 'Kelas' : 'Kelas / bagian'}<input name="group" defaultValue={person.group_name} placeholder={isStudent ? 'Contoh: 9' : 'Contoh: VII A atau Guru Matematika'} maxLength={60} required/></label>
      {isStudent && <div className="student-form-fields">
        <label>NISN<input name="nisn" defaultValue={profile?.nisn} inputMode="numeric" maxLength={10} pattern="[0-9]{10}" title="Tepat 10 digit"/></label>
        <label>Jenis Kelamin<select name="gender" defaultValue={profile?.gender ?? ''}><option value="">Belum diisi</option><option value="L">L — Laki-laki</option><option value="P">P — Perempuan</option></select></label>
        <label>Kelas Paralel<input name="class_parallel" defaultValue={profile?.class_parallel} placeholder="Contoh: 3 atau A" maxLength={60}/></label>
        <label>NIK Siswa<input name="nik" defaultValue={profile?.nik} inputMode="numeric" maxLength={16} pattern="[0-9]{16}" title="Tepat 16 digit"/></label>
        <label>Tempat Lahir<input name="birth_place" defaultValue={profile?.birth_place} maxLength={100}/></label>
        <label>Tanggal Lahir<input type="date" name="birth_date" defaultValue={profile?.birth_date} min="0001-01-01" max="9999-12-31"/></label>
        <label className="student-address-field">Alamat Rumah<textarea name="home_address" defaultValue={profile?.home_address} maxLength={500} rows={2}/></label>
      </div>}
      <div className="modal-actions">
        <button type="button" className="btn secondary" onClick={onClose}>Batal</button>
        <button className="btn" disabled={busy}>{busy ? 'Menyimpan…' : 'Simpan data'}<Check size={17}/></button>
      </div>
    </form>
  </div>;
}
