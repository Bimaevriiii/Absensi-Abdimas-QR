import { env } from 'cloudflare:workers';
export const dynamic = 'force-dynamic';
const defaults={school:'Sekolah Kita',student_time:'07:00',teacher_time:'06:45',timezone:'Asia/Jakarta'};
function db(){if(!env.DB)throw new Error('Database belum tersedia.');return env.DB;}
function dayAt(date:Date,tz:string){return new Intl.DateTimeFormat('en-CA',{timeZone:tz,year:'numeric',month:'2-digit',day:'2-digit'}).format(date);}
function minutesAt(date:Date,tz:string){const p=new Intl.DateTimeFormat('en-GB',{timeZone:tz,hour:'2-digit',minute:'2-digit',hourCycle:'h23'}).format(date).split(':');return +p[0]*60+(+p[1]);}
async function config(){return await db().prepare('SELECT * FROM settings WHERE id=?').bind('main').first()||defaults;}
function result(data:any,status=200){return Response.json(data,{status,headers:{'Cache-Control':'no-store'}})}
function auth(req:Request){if(process.env.NODE_ENV==='production'&&!req.headers.get('oai-authenticated-user-id'))throw new Error('Silakan masuk untuk mengakses absensi.');}
export async function GET(req:Request){try{auth(req);const cfg:any=await config();const {results:p}=await db().prepare('SELECT * FROM people ORDER BY name').all();const {results:a}=await db().prepare('SELECT * FROM attendance ORDER BY arrived DESC').all();return result({people:p,attendance:a,settings:cfg,today:dayAt(new Date(),cfg.timezone),serverTime:new Date().toISOString()});}catch(e:any){return result({error:e.message},400)}}
export async function POST(req:Request){try{auth(req);const b:any=await req.json();const cfg:any=await config();
 if(b.action==='person'){
 const name=String(b.name||'').trim(),number=String(b.number||'').trim(),role=b.role,group=String(b.group||'').trim();
 if(!name||name.length>100||!number||number.length>40||!group||group.length>60||!['murid','guru'].includes(role))return result({error:'Lengkapi nama, nomor induk, jenis, dan kelas/bagian dengan benar.'},400);
 const duplicate=await db().prepare('SELECT id FROM people WHERE number=? AND id!=?').bind(number,b.id||'').first();if(duplicate)return result({error:'Nomor induk sudah terdaftar.'},409);
 if(b.id){const ex=await db().prepare('SELECT id FROM people WHERE id=?').bind(b.id).first();if(!ex)return result({error:'Data tidak ditemukan.'},404);await db().prepare('UPDATE people SET name=?, number=?,role=?,group_name=? WHERE id=?').bind(name,number,role,group,b.id).run();}
 else await db().prepare('INSERT INTO people(id,token,name,number,role,group_name) VALUES(?,?,?,?,?,?)').bind(crypto.randomUUID(),crypto.randomUUID(),name,number,role,group).run();return result({message:b.id?'Data berhasil diperbarui.':'Data baru dan kartu QR berhasil dibuat.'});
 }
 if(b.action==='active'){await db().prepare('UPDATE people SET active=? WHERE id=?').bind(b.active?1:0,b.id).run();return result({message:b.active?'Data diaktifkan.':'Data dinonaktifkan. Riwayat absensi tetap tersimpan.'});}
 if(b.action==='lookup'||b.action==='scan'){
 if(b.action==='scan'&&!['masuk','pulang'].includes(b.mode))return result({error:'Pilih absensi masuk atau pulang.'},400);
 const token=String(b.token||'').trim().replace(/^HADIR:/,'');const p:any=await db().prepare('SELECT * FROM people WHERE token=? AND active=1').bind(token).first();if(!p)return result({error:'QR tidak terdaftar atau kartu sudah nonaktif.'},404);
 const now=new Date(),day=dayAt(now,cfg.timezone);const old:any=await db().prepare('SELECT * FROM attendance WHERE person_id=? AND day=?').bind(p.id,day).first();
 if(b.action==='lookup')return result({person:p,record:old});
 if(b.mode==='pulang'){if(!old)return result({error:'Belum ada absensi masuk hari ini.'},400);if(old.departed)return result({error:'Absensi pulang hari ini sudah tercatat.'},409);await db().prepare('UPDATE attendance SET departed=? WHERE id=? AND departed IS NULL').bind(now.toISOString(),old.id).run();return result({message:'Absensi pulang berhasil dicatat.',person:p});}
 if(old)return result({error:'Sudah absen masuk hari ini. Data tidak digandakan.'},409);
 const cutoff=String(p.role==='guru'?cfg.teacher_time:cfg.student_time).split(':');const late=Math.max(0,minutesAt(now,cfg.timezone)-(+cutoff[0]*60+(+cutoff[1])));
 try{await db().prepare('INSERT INTO attendance(id,person_id,day,arrived,status,late_minutes) VALUES(?,?,?,?,?,?)').bind(crypto.randomUUID(),p.id,day,now.toISOString(),late?'Terlambat':'Tepat waktu',late).run();}catch(e){if(String(e).includes('UNIQUE'))return result({error:'Absensi hari ini sudah tercatat.'},409);throw e;}
 return result({message:late?'Kehadiran tercatat · terlambat '+late+' menit.':'Kehadiran tepat waktu berhasil dicatat.',person:p});
 }
 if(b.action==='settings'){
 if(!String(b.school||'').trim()||String(b.school).length>100||!/^([01]\d|2[0-3]):[0-5]\d$/.test(b.student_time)||!/^([01]\d|2[0-3]):[0-5]\d$/.test(b.teacher_time)||!['Asia/Jakarta','Asia/Makassar','Asia/Jayapura'].includes(b.timezone))return result({error:'Periksa nama sekolah, jam masuk, dan zona waktu.'},400);
 await db().prepare('INSERT INTO settings(id,school,student_time,teacher_time,timezone) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET school=excluded.school,student_time=excluded.student_time,teacher_time=excluded.teacher_time,timezone=excluded.timezone').bind('main',b.school.trim(),b.student_time,b.teacher_time,b.timezone).run();return result({message:'Pengaturan berhasil disimpan.'});
 }
 if(b.action==='demo'){
 const names=['Aditya Pratama','Alya Putri','Bima Ramadhan','Citra Maharani','Daffa Saputra','Fadila Zahra','Gilang Nugraha','Hana Azzahra','Irfan Maulana','Kirana Putri','Luthfi Hakim','Nadia Safira','Rina Wulandari','Ahmad Fauzi','Dewi Lestari'];
 const batch=names.map((n,i)=>db().prepare('INSERT OR IGNORE INTO people(id,token,name,number,role,group_name,demo) VALUES(?,?,?,?,?,?,1)').bind('demo-'+i,crypto.randomUUID(),n,'CONTOH-'+(i+1),i<12?'murid':'guru',i<12?['VII A','VII B','VIII A','IX A'][i%4]:['Matematika','Bahasa Indonesia','IPA'][i-12]));await db().batch(batch);return result({message:'15 data contoh ditambahkan. Belum ada absensi yang dibuat.'});
 }
 if(b.action==='clearDemo'){await db().batch([db().prepare('DELETE FROM attendance WHERE person_id IN (SELECT id FROM people WHERE demo=1)'),db().prepare('DELETE FROM people WHERE demo=1')]);return result({message:'Data contoh beserta absensi contohnya dihapus.'});}
 return result({error:'Tindakan tidak dikenal.'},400);
 }catch(e:any){return result({error:e.message||'Terjadi kesalahan. Silakan coba kembali.'},400)}}
