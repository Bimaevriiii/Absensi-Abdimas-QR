# Hadir — Absensi Sekolah

Cabang `go-pwa-dokploy` berisi migrasi aplikasi absensi QR MTs Amal Bakti dari Cloudflare Worker/D1 ke **Go, PostgreSQL, dan React PWA**. File `compose.yaml` dapat dijalankan dengan Docker Compose atau dipakai sebagai sumber aplikasi Docker Compose di Dokploy. Frontend memakai path relatif `/api` pada domain yang sama dengan API.

## Ringkasan fitur dan tahap

| Tahap | Hasil |
|---|---|
| 01 | API Go, PostgreSQL, migrasi SQL, login dengan sesi, dan tiga peran |
| 02 | Frontend React/Vite dengan tampilan prototipe, integrasi API, dan PWA online |
| 03 | Izin/sakit/alpa, input manual, koreksi pengelola dengan audit, dan jam pulang |
| 04 | Compose Go/PostgreSQL/frontend, proxy Nginx, panduan Dokploy, backup, dan smoke test |

| Peran | Hak utama |
|---|---|
| Pengelola | Seluruh layar, pengaturan sekolah, akun, koreksi absensi, dan audit |
| Operator | Pindai, direktori, kartu QR, rekap, data contoh, serta status manual; pengaturan hanya baca |
| Petugas gerbang | Pindai masuk/pulang dan input izin/sakit/alpa **hari ini** setelah mencocokkan identitas |

Murid dan guru dapat dicatat masuk lalu pulang satu kali per tanggal sekolah. QR berisi token statis, jadi petugas harus mencocokkan identitas pemegang kartu. Status `Tepat waktu`/`Terlambat` dihitung server dari jam masuk. Status `Izin`, `Sakit`, dan `Alpa` dibuat manual dengan alasan; peserta yang belum memiliki catatan tetap `Belum absen`, bukan otomatis alpa. Pengelola dapat menambah kehadiran yang terlewat atau mengoreksi catatan dengan alasan dan riwayat sebelum/sesudah. Rekap dan CSV menampilkan status serta keterangan.

PWA menyediakan manifest, ikon, dan service worker. **Absensi tetap memerlukan koneksi jaringan**; tidak ada cache data akun/absensi ataupun antrean scan offline.

## Struktur proyek

| Path | Isi |
|---|---|
| `cmd/hadir`, `internal/app` | API Go, otorisasi, logika absensi, migrasi, dan tes |
| `frontend` | React/Vite, aset PWA, Dockerfile, dan konfigurasi Nginx |
| `compose.yaml` | Layanan `db`, `api`, dan `web` untuk Dokploy/Compose |
| `compose.local.yaml` | Port loopback dan cookie HTTP khusus uji lokal |
| `deploy/.env.example` | Template variabel PostgreSQL untuk Compose |
| `deploy/smoke.sh` | Pemeriksaan HTTP tanpa membuat data aplikasi |

Backend mengekspos `GET /healthz` untuk proses dan `GET /readyz` untuk koneksi database. Hanya `web` yang perlu menerima domain publik. API dan PostgreSQL tidak menerbitkan port host pada `compose.yaml`.

## Uji deployment tanpa Dokploy

Butuh **Docker Engine dan Docker Compose plugin** pada mesin uji. Jika belum terpasang di Debian, ikuti [panduan instalasi resmi Docker](https://docs.docker.com/engine/install/debian/). Periksa dengan `docker version` dan `docker compose version`.

Setelah branch ini dipush, clone langsung branch migrasi:

```bash
git clone --single-branch --branch go-pwa-dokploy git@github.com:Bimaevriiii/Absensi-Abdimas-QR.git hadir
cd hadir
cp deploy/.env.example .env
openssl rand -hex 32
```

Salin hasil `openssl rand` ke nilai `POSTGRES_PASSWORD` dalam `.env`. Jangan gunakan teks contoh. Jika source sudah tersedia di host, masuk ke foldernya tanpa melakukan clone lagi.

Bangun dan jalankan tiga container:

```bash
docker compose -f compose.yaml -f compose.local.yaml config --quiet
docker compose -f compose.yaml -f compose.local.yaml up -d --build --wait
docker compose -f compose.yaml -f compose.local.yaml ps
sh deploy/smoke.sh http://127.0.0.1:18088
```

Status `db`, `api`, dan `web` seharusnya sehat. Output terakhir smoke test adalah `Smoke test passed`. Buka `http://127.0.0.1:18088` pada browser **di mesin Docker**. Pada host Docker jarak jauh, jalankan smoke test di host itu dan gunakan terowongan SSH dari komputer Anda:

```bash
ssh -L 18088:127.0.0.1:18088 USER@HOST_DOCKER
```

Selama koneksi SSH terbuka, buka `http://127.0.0.1:18088` di browser komputer Anda. Port Compose hanya terikat ke loopback sehingga tidak terbuka langsung ke jaringan.

### Buat pengelola pertama

Masuk ke container API dari folder proyek di host Docker:

```bash
docker compose -f compose.yaml -f compose.local.yaml exec api sh
```

Di shell container, jalankan ini **sekali** dengan email uji dan kata sandi minimal 12 karakter:

```sh
printf 'Email admin: '; IFS= read -r ADMIN_EMAIL
printf 'Kata sandi admin: '; stty -echo; IFS= read -r ADMIN_PASSWORD; stty echo; printf '\n'
export ADMIN_EMAIL ADMIN_PASSWORD
/app/hadir create-admin
unset ADMIN_EMAIL ADMIN_PASSWORD
exit
```

Login melalui browser, lalu uji dengan data sintetis: buat murid/guru, lihat kartu QR, catat masuk dan pulang, input status manual, koreksi sebagai pengelola, periksa audit, rekap, dan unduh CSV. Coba peran operator dan petugas untuk memastikan pembatasan menu dan API. Akun dapat dibuat pengelola dari panel aplikasi.

Di **Murid & guru → Murid**, pengelola dan operator dapat mengisi biodata siswa. NIS Lokal, Nama Siswa, dan Kelas wajib. NISN, Jenis Kelamin, Kelas Paralel, Tempat dan Tanggal Lahir, NIK Siswa, serta Alamat Rumah dapat dilengkapi kemudian. Migrasi memakai nomor induk lama sebagai NIS Lokal dan mempertahankan kelas serta QR murid yang sudah terdaftar.

### Diagnostik dan menghentikan uji

```bash
docker compose -f compose.yaml -f compose.local.yaml ps
docker compose -f compose.yaml -f compose.local.yaml logs --tail=100 db api web
curl -i http://127.0.0.1:18088/readyz
docker compose -f compose.yaml -f compose.local.yaml down
```

`down` menghentikan stack dan mempertahankan named volume PostgreSQL. Menambahkan `-v` akan menghapus volume beserta data uji. Layanan `api` menjalankan migrasi otomatis sebelum mulai melayani permintaan.

Browser memperlakukan `localhost`/`127.0.0.1` sebagai [secure context](https://developer.mozilla.org/en-US/docs/Web/Security/Defenses/Secure_Contexts), sehingga service worker dan kamera dapat diuji pada perangkat yang membuka alamat lokal itu. Ponsel lain tidak dapat mengakses port loopback tersebut. Untuk kamera/PWA pada ponsel, gunakan domain **HTTPS** yang dapat diakses ponsel; HTTP ke alamat IP LAN biasa tidak cukup untuk [akses kamera browser](https://developer.mozilla.org/en-US/docs/Web/API/MediaDevices/getUserMedia).

## Deploy dengan Dokploy

1. Push branch `go-pwa-dokploy` ke repositori yang dapat dibaca Dokploy, lalu buat aplikasi **Docker Compose** dari branch itu. Pilih file `compose.yaml` pada root repositori; jangan gunakan `compose.local.yaml` untuk produksi. Mode Docker Stack tidak cocok karena konfigurasi ini memakai `build`.
2. Isi `POSTGRES_DB`, `POSTGRES_USER`, dan `POSTGRES_PASSWORD` di tab Environment berdasarkan `deploy/.env.example`. Gunakan kata sandi acak panjang dan simpan di pengelola rahasia. Compose mengirim variabel tersebut secara eksplisit ke container.
3. Pada tab Domains, arahkan domain sekolah **hanya** ke service `web`, container port `8080`, path `/`. Aktifkan HTTPS dengan sertifikat yang valid. Pastikan Preview Compose tetap menghubungkan `web` dan `api` melalui jaringan `app`. Perubahan domain Compose memerlukan deploy ulang menurut [panduan Dokploy](https://docs.dokploy.com/docs/core/docker-compose/domains).
4. Deploy, tunggu semua layanan sehat, buka `https://DOMAIN/readyz`, lalu jalankan `sh deploy/smoke.sh https://DOMAIN`. Buat pengelola pertama melalui terminal service `api` memakai perintah di atas. Di produksi, cookie sesi memakai `Secure`.
5. Sebelum memakai data sekolah, uji di lokasi: HTTPS dan instalasi PWA di ponsel, izin kamera, QR cetak, pemindai USB, masuk/pulang, status manual, koreksi/audit, CSV, dan pemulihan backup.

## Environment, data, dan backup

`deploy/.env.example` adalah satu-satunya template Compose. File `.env` sebenarnya diabaikan Git. `POSTGRES_DB` dan `POSTGRES_USER` menentukan database/user awal; `POSTGRES_PASSWORD` wajib diubah. Compose mengatur `DB_HOST=db`, `HTTP_ADDR=0.0.0.0:8080`, dan `COOKIE_SECURE=true` untuk API. Overlay lokal mengganti cookie menjadi non-Secure agar login pada HTTP loopback bekerja.

PostgreSQL menyimpan data pada named volume `postgres_data`. Jadwalkan backup sebelum pembaruan dan harian. Contoh backup logis dari checkout yang menjalankan Compose:

```bash
umask 077
docker compose exec -T db sh -c 'export PGPASSWORD="$POSTGRES_PASSWORD"; exec pg_dump -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "hadir-$(date -u +%Y%m%dT%H%M%SZ).dump"
```

Simpan file `.dump` di luar server dan uji pemulihan ke database **baru**. Contoh pemulihan ke `hadir_restore`:

```bash
docker compose stop web api
docker compose exec -T db sh -c 'export PGPASSWORD="$POSTGRES_PASSWORD"; exec createdb -h 127.0.0.1 -U "$POSTGRES_USER" hadir_restore'
docker compose exec -T db sh -c 'export PGPASSWORD="$POSTGRES_PASSWORD"; exec pg_restore -h 127.0.0.1 -U "$POSTGRES_USER" -d hadir_restore --no-owner --exit-on-error' < hadir-BACKUP.dump
```

Periksa hasilnya sebelum mengganti `POSTGRES_DB` ke database baru dan melakukan deploy ulang. Database lama tetap ada di volume sebagai jalur balik. Untuk aplikasi yang dijalankan Dokploy, gunakan nama project dan path Compose yang ditampilkan di konfigurasi Advanced Dokploy saat menjalankan perintah CLI. Dokploy juga mendukung [backup named volume](https://docs.dokploy.com/docs/core/volume-backups). Migrasi SQL berjalan maju saat API mulai; buat backup sebelum menaikkan versi.

## Pengembangan tanpa Docker

Persyaratan lokal: Go 1.27+, PostgreSQL 17, Node.js 22.13+, dan npm. Buat database kosong, lalu jalankan dari root proyek:

```bash
go mod download
DATABASE_URL='postgres://hadir:YOUR_PASSWORD@127.0.0.1:5432/hadir?sslmode=disable' go run ./cmd/hadir migrate
DATABASE_URL='postgres://hadir:YOUR_PASSWORD@127.0.0.1:5432/hadir?sslmode=disable' ADMIN_EMAIL='admin@example.test' ADMIN_PASSWORD='REPLACE_WITH_LONG_UNIQUE_PASSWORD' go run ./cmd/hadir create-admin
DATABASE_URL='postgres://hadir:YOUR_PASSWORD@127.0.0.1:5432/hadir?sslmode=disable' COOKIE_SECURE=false go run ./cmd/hadir serve
```

Di terminal lain:

```bash
cd frontend
npm ci
API_PROXY_TARGET=http://127.0.0.1:8080 npm run dev
```

Vite memakai proxy `/api` hanya saat development/preview. Build produksi memanggil `/api` pada origin yang sama dan disajikan Nginx. Backend juga menerima `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, dan `DB_SSLMODE` bila `DATABASE_URL` tidak diisi.

## API dan aturan akses

| Endpoint | Kegunaan |
|---|---|
| `POST /api/auth/login`, `POST /api/auth/logout`, `GET /api/auth/me` | Sesi pengguna |
| `GET /api/hadir` | Data peserta, absensi, pengaturan, dan tanggal sekolah sesuai peran |
| `GET /api/students` | Daftar biodata murid dengan ID peserta, hanya pengelola dan operator |
| `POST /api/hadir` | Aksi `person`, `active`, `lookup`, `lookupNumber`, `scan`, `settings`, `demo`, `clearDemo`, `manual`, atau `correct` |
| `GET/POST /api/users` | Kelola akun, hanya pengelola |
| `GET /api/audit` | Riwayat koreksi, hanya pengelola |
| `GET /healthz`, `GET /readyz` | Status proses dan database |

`lookupNumber` mencari peserta aktif berdasarkan NIS Lokal, NISN, atau NIP. Jika nomor cocok dengan dua peserta, API mengembalikan `409 ambiguous_number`. Ulangi pencarian dengan `number_type` bernilai `number` untuk NIS Lokal atau NIP, atau `nisn` untuk NISN. Aksi `person` mendukung objek opsional `student` dengan `nisn`, `gender`, `class_parallel`, `birth_place`, `birth_date`, `nik`, dan `home_address`. Tanggal lahir memakai format `YYYY-MM-DD`. Jika objek `student` tidak dikirim, biodata yang telah tersimpan dipertahankan. `manual` menerima `person_id`, `day`, `status` (`Izin`/`Sakit`/`Alpa`), dan `reason` sepanjang 5–500 karakter; petugas hanya boleh mencatat hari ini. Pengelola juga dapat menambah kehadiran terlewat dengan `status: "Hadir"` dan waktu `arrived`. Aksi `correct` hanya untuk pengelola, mensyaratkan `reason` dan perubahan nyata, serta mencatat snapshot audit. `GET /api/audit?attendance_id=...` memfilter satu catatan. Jam pulang harus setelah masuk dan tidak boleh di masa depan. Tanggal mengikuti zona waktu sekolah.

Respons `401` berarti belum login, `403` berarti peran tidak berhak, `409` berarti konflik/duplikasi, dan `400` berarti input salah. API mengirim `Cache-Control: no-store`. Kata sandi di-hash dengan Argon2id; sesi menggunakan cookie `HttpOnly`, `SameSite=Strict`, dan `Secure` di produksi. POST dari `Origin` lain ditolak. Seluruh otorisasi diperiksa di API walaupun menu frontend juga dibatasi.

## Verifikasi dan batas saat ini

```bash
go test ./...
go vet ./...
cd frontend && npm ci && npm run build
```

Tes PostgreSQL tambahan memakai `TEST_DATABASE_URL` dan **mengosongkan database bernama `hadir_stage1_test`**; jangan arahkan tes ini ke database aplikasi. Contoh: `TEST_DATABASE_URL='postgres://hadir:PASS@127.0.0.1:5432/hadir_stage1_test?sslmode=disable' go test -count=1 -v ./internal/app`.

Pada 15 November 2026, unit/integrasi Go, build frontend, validasi Compose, smoke test lokal Go + PostgreSQL + Nginx, login dan service worker di Chromium, serta backup/restore sintetis lulus. **Image Docker, server Dokploy, domain HTTPS nyata, kamera/USB fisik, dan instalasi PWA pada ponsel belum diuji di workspace ini** karena Docker Engine dan perangkat tersebut tidak tersedia. Pengujian Compose pada host Docker serta uji penerimaan di lokasi tetap perlu dilakukan sebelum operasional.
