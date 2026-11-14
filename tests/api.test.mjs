import assert from 'node:assert/strict';
import {DatabaseSync} from 'node:sqlite';
import {readFileSync,writeFileSync,unlinkSync} from 'node:fs';
import {build} from 'esbuild';
import QRCode from 'qrcode';
import jsQR from 'jsqr';
import {PNG} from 'pngjs';
const sqlite=new DatabaseSync(':memory:');
sqlite.exec(readFileSync('drizzle/0000_mushy_micromacro.sql','utf8'));
function statement(sql,args=[]){return {bind(...values){return statement(sql,values)},async first(){return sqlite.prepare(sql).get(...args)||null},async all(){return {results:sqlite.prepare(sql).all(...args)}},async run(){return sqlite.prepare(sql).run(...args)}}}
globalThis.__TEST_ENV={DB:{prepare:statement,async batch(items){return Promise.all(items.map(x=>x.run()))}}};
const bundled=await build({entryPoints:['app/api/hadir/route.ts'],bundle:true,write:false,format:'esm',platform:'node',plugins:[{name:'test-env',setup(b){b.onResolve({filter:/^cloudflare:workers$/},()=>({path:'env',namespace:'test'}));b.onLoad({filter:/.*/,namespace:'test'},()=>({contents:'export const env=globalThis.__TEST_ENV'}))}}]});
const tmp=new URL('./.route-test.mjs',import.meta.url);writeFileSync(tmp,bundled.outputFiles[0].text);
const {GET,POST}=await import(tmp.href);unlinkSync(tmp);
async function call(body){const req=new Request('https://example.test/api/hadir',{method:body?'POST':'GET',headers:{'Content-Type':'application/json','oai-authenticated-user-id':'test-owner'},...(body?{body:JSON.stringify(body)}:{})});const r=await (body?POST(req):GET(req));return {status:r.status,body:await r.json()}}
process.env.NODE_ENV='production';
assert.equal((await GET(new Request('https://example.test/api/hadir'))).status,400);
assert.equal((await call()).status,200);
assert.equal((await call({action:'demo'})).status,200);
const people=(await call()).body.people;assert.equal(people.length,15);assert.equal(new Set(people.map(p=>p.token)).size,15);
const student=people.find(p=>p.role==='murid'),teacher=people.find(p=>p.role==='guru');
for(const p of [student,teacher]){
assert.equal((await call({action:'lookup',token:'HADIR:'+p.token})).body.person.id,p.id);
assert.equal((await call({action:'scan',token:p.token,mode:'pulang'})).status,400);
assert.equal((await call({action:'scan',token:p.token,mode:'masuk'})).status,200);
assert.equal((await call({action:'scan',token:p.token,mode:'masuk'})).status,409);
assert.equal((await call({action:'scan',token:p.token,mode:'pulang'})).status,200);
assert.equal((await call({action:'scan',token:p.token,mode:'pulang'})).status,409);
}
assert.equal((await call()).body.attendance.length,2);
assert.equal((await call({action:'lookup',token:'unknown'})).status,404);
assert.equal((await call({action:'person',name:'',number:'QA',group:'A',role:'murid'})).status,400);
assert.equal((await call({action:'person',name:'QA',number:student.number,group:'A',role:'murid'})).status,409);
assert.equal((await call({action:'person',id:student.id,name:student.name,number:student.number,group:'VII Z',role:'murid'})).status,200);
assert.equal((await call({action:'lookup',token:student.token})).body.person.group_name,'VII Z');
await call({action:'active',id:student.id,active:false});
assert.equal((await call({action:'lookup',token:student.token})).status,404);
assert.equal((await call({action:'settings',school:'Sekolah',student_time:'99:99',teacher_time:'06:45',timezone:'Asia/Jakarta'})).status,400);
assert.equal((await call({action:'scan',token:teacher.token,mode:'invalid'})).status,400);
await call({action:'clearDemo'});assert.equal((await call()).body.people.length,0);assert.equal((await call()).body.attendance.length,0);
const payload='HADIR:64fd417c-28a5-4566-991c-7d17fba2982f';const img=PNG.sync.read(await QRCode.toBuffer(payload,{width:400,margin:3}));assert.equal(jsQR(new Uint8ClampedArray(img.data),img.width,img.height)?.data,payload);
console.log('PASS: protected access; 15 unique QRs; student + teacher check-in/out; duplicate prevention; invalid code/input; edit preserves QR; disable card; settings validation; demo cleanup; QR encoding/decoding.');
sqlite.close();
