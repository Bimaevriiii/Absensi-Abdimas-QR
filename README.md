# Hadir — Absensi Online Murid dan Guru

Kode sumber website yang dibuat pada percakapan ini.

## Fitur

Dashboard; tambah/edit/aktif/nonaktif murid dan guru; QR unik; cetak kartu;
unduh QR PNG; kamera/unggah gambar/scanner USB; verifikasi identitas;
absensi masuk/pulang; pencegahan absen ganda; keterlambatan; filter rekap;
ekspor CSV; pengaturan nama sekolah, jam masuk dan zona waktu.
Data contoh tersedia melalui tombol pada halaman Pengaturan.

## Teknologi dan isi paket

React + TypeScript + CSS, Vinext/Vite (App Router), Cloudflare Worker,
Cloudflare D1 (SQLite), qrcode, jsqr dan lucide-react.
Ini kode versi asli yang masih memakai infrastruktur Sites/Cloudflare.
Bukan aplikasi PHP/XAMPP dan belum memakai login sekolah mandiri.

Kode aplikasi, konfigurasi, package-lock.json, migrasi SQL dan tes disertakan.
Database aktual, absensi pengujian, node_modules, hasil build, Git dan
kredensial tidak disertakan. ID proyek hosting dihapus dari salinan ekspor.
Ekspor ini tidak mengubah website terbit.

## Menjalankan lokal di Windows / PowerShell

Prasyarat: Node.js >= 22.13.0 dan npm.
Ekstrak ZIP, buka PowerShell pada folder hadir-sekolah berisi package.json.

1. Instal dependensi:

```powershell
npm ci
```

2. Build (menghasilkan konfigurasi Worker lokal):

```powershell
npm run build
```

3. Buat tabel pada database LOKAL yang masih kosong:

```powershell
node --import ./scripts/sites-env.mjs ./node_modules/wrangler/bin/wrangler.js d1 execute DB --local --config dist/server/wrangler.json --persist-to .wrangler/state --file drizzle/0000_mushy_micromacro.sql
```

Migrasi awal dijalankan satu kali. Jangan diulang jika tabel sudah ada.
Perintah --local tidak mengubah database website terbit.

4. Jalankan mode pengembangan:

```powershell
npm run dev
```

Buka alamat aktual di terminal, biasanya http://localhost:5173.
Biarkan terminal terbuka. Ctrl+C menghentikan server.
Mode pengembangan hanya untuk uji lokal, bukan untuk dibuka sebagai server publik.

5. Buka Pengaturan untuk nama sekolah/jam masuk. Tambahkan murid/guru,
buka Kartu QR lalu cetak/unduh, dan gunakan Pindai absensi di gerbang.
Untuk mencoba tanpa data asli, gunakan Pengaturan > Tambahkan data contoh.
Pembuatan data contoh tidak otomatis membuat absensi.
Kamera memerlukan izin browser; pada ponsel gunakan HTTPS.

## File penting

- app/page.tsx: antarmuka dan alur pengguna.
- app/globals.css: desain desktop, responsif dan kartu cetak.
- app/api/hadir/route.ts: API direktori, QR, absensi dan pengaturan.
- db/schema.ts: struktur tabel.
- drizzle/: migrasi SQL dan metadata.
- worker/index.ts: entrypoint Worker.
- vite.config.ts: build dan binding database.
- tests/api.test.mjs: tes API dan QR.
- .openai/hosting.json: binding, tanpa ID proyek terbit.
- README-STARTER.md: dokumentasi teknis bawaan starter.

## Pemeriksaan kode

```powershell
npx tsc --noEmit
node tests/api.test.mjs
```

Tes API memakai SQLite sementara di memori dan tidak mengubah data sekolah.
Tes memeriksa akses API, QR unik, masuk/pulang murid dan guru,
penolakan absen ganda, validasi, edit, nonaktif dan pembersihan data contoh.

## Sebelum hosting mandiri

Paket ini belum dikonversi menjadi versi dengan login admin/guru mandiri.
Untuk produksi di luar Sites, diperlukan:

1. Cloudflare Worker + D1 dengan binding DB dan migrasi, atau port API/database.
2. Login/sesi serta otorisasi admin/petugas di sisi server.
   Versi asli mengandalkan gerbang privat Sites dan header
   oai-authenticated-user-id. Header ini tidak dapat dipercaya begitu saja
   pada hosting lain: pengunjung bisa memalsukannya bila server tidak melindungi
   jalur masuk. Jangan menghapus pemeriksaan akses dan membuka API ke publik.
3. Jalur /signin-with-chatgpt milik platform Sites. Simulasi login lokal
   pada starter bukan login produksi atau akun sekolah.
4. HTTPS, akun petugas, pencadangan database dan uji kamera/printer sekolah.

Mengekstrak ZIP ke hosting PHP biasa belum akan menjalankan aplikasi ini.
Untuk mencoba di komputer, gunakan petunjuk PowerShell di atas.

## Catatan perilaku

- QR berisi token acak, bukan nama, NIS/NIP, alamat atau tanggal lahir.
- QR tetap dapat disalin; petugas perlu mencocokkan identitas pemegang kartu.
- Keterlambatan dihitung server menurut zona waktu dan aturan saat absen.
- Belum absen tidak otomatis berarti alpa; izin/sakit belum disediakan.
- Nama/kelas dalam laporan mengikuti direktori terkini. Waktu dan status lama
  tetap tersimpan; belum ada snapshot riwayat kelas/nama.
- Menghapus data contoh juga menghapus catatan absensi data contoh.
- Import Excel massal, portal orang tua dan notifikasi WhatsApp belum termasuk.
- Lisensi starter/plugin tersedia di scripts/vite/sites-vite-plugin.LICENSE
  dan vendor/. Paket pihak ketiga mengikuti lisensi masing-masing.
