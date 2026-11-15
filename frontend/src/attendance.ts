export type Person = { id: string; token: string; name: string; number: string; role: string; group_name: string; active: number; demo: number };
export type RecordItem = { id: string; person_id: string; day: string; arrived: string | null; departed: string | null; status: string; late_minutes: number; note: string; source: string };
export type SchoolSettings = { school: string; student_time: string; teacher_time: string; student_departure_time: string; teacher_departure_time: string; timezone: string };

const offsets: Record<string, string> = {
  'Asia/Jakarta': '+07:00', 'Asia/Makassar': '+08:00', 'Asia/Jayapura': '+09:00',
};

export function schoolInputToISO(value: string, timezone: string): string {
  const parsed = new Date(value + (offsets[timezone] || '+07:00'));
  if (Number.isNaN(parsed.getTime())) throw Error('Waktu yang dipilih tidak valid.');
  return parsed.toISOString();
}

export function isoToSchoolInput(value: string | null, timezone: string): string {
  if (!value) return '';
  const parts = new Intl.DateTimeFormat('sv-SE', {
    timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
  }).formatToParts(new Date(value));
  const part = (name: string) => parts.find(item => item.type === name)?.value || '';
  return `${part('year')}-${part('month')}-${part('day')}T${part('hour')}:${part('minute')}`;
}

export function schoolTime(value: string | null | undefined, timezone: string): string {
  if (!value) return '—';
  return new Intl.DateTimeFormat('id-ID', { timeZone: timezone, hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(new Date(value));
}
