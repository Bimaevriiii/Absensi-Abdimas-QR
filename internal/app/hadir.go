package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Minimum time between masuk and pulang scans.
const minPulangGap = 5 * time.Minute

type actionBody struct {
	Action     string          `json:"action"`
	ID         string          `json:"id"`
	PersonID   string          `json:"person_id"`
	Name       string          `json:"name"`
	Number     string          `json:"number"`
	NumberType string          `json:"number_type"`
	Student    *studentProfile `json:"student"`
	Role       string          `json:"role"`
	Group      string          `json:"group"`
	Active     *bool           `json:"active"`
	Token      string          `json:"token"`
	Mode       string          `json:"mode"`
	Day        string          `json:"day"`
	Status     string          `json:"status"`
	Arrived    string          `json:"arrived"`
	Departed   string          `json:"departed"`
	Note       string          `json:"note"`
	Reason     string          `json:"reason"`
	Settings
}

func (s *Server) hadir(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireUser(w, r, "pengelola", "operator", "petugas")
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		s.hadirGet(w, r, actor)
		return
	}
	var b actionBody
	if err := decodeJSON(r, &b); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Data JSON tidak valid."})
		return
	}
	switch b.Action {
	case "demo", "clearDemo":
		if actor.Role != "pengelola" {
			writeJSON(w, 403, map[string]string{"error": "Hanya pengelola yang dapat mengelola data contoh."})
			return
		}
	case "person", "active":
		if actor.Role == "petugas" {
			writeJSON(w, 403, map[string]string{"error": "Akun Anda tidak memiliki izin untuk tindakan ini."})
			return
		}
	case "settings":
		if actor.Role != "pengelola" {
			writeJSON(w, 403, map[string]string{"error": "Hanya pengelola yang dapat mengubah pengaturan."})
			return
		}
	case "correct":
		if actor.Role != "pengelola" {
			writeJSON(w, 403, map[string]string{"error": "Hanya pengelola yang dapat mengoreksi absensi."})
			return
		}
	case "lookup", "lookupNumber", "scan", "manual":
	default:
		writeJSON(w, 400, map[string]string{"error": "Tindakan tidak dikenal."})
		return
	}
	switch b.Action {
	case "person":
		s.personAction(w, r, b)
	case "active":
		s.activeAction(w, r, b)
	case "lookup", "scan":
		s.scanAction(w, r, actor, b)
	case "lookupNumber":
		s.lookupNumberAction(w, r, b)
	case "manual":
		s.manualAction(w, r, actor, b)
	case "correct":
		s.correctAction(w, r, actor, b)
	case "settings":
		s.settingsAction(w, r, b)
	case "demo":
		s.demoAction(w, r)
	case "clearDemo":
		s.clearDemoAction(w, r)
	}
}

func (s *Server) hadirGet(w http.ResponseWriter, r *http.Request, actor User) {
	cfg, err := readSettings(r.Context(), s.db)
	if err != nil {
		s.fail(w, err)
		return
	}
	day, _, err := dayAndMinutes(s.now(), cfg.Timezone)
	if err != nil {
		s.fail(w, err)
		return
	}
	// Optional day filter; default is the last 120 school days.
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if from == "" {
		t, _ := time.Parse("2006-01-02", day)
		from = t.AddDate(0, 0, -120).Format("2006-01-02")
	}
	if to == "" {
		to = "9999-12-31"
	}
	for _, d := range []string{from, to} {
		if t, err := time.Parse("2006-01-02", d); err != nil || t.Format("2006-01-02") != d {
			writeJSON(w, 400, map[string]string{"error": "Parameter from/to harus berformat YYYY-MM-DD."})
			return
		}
	}
	people := []Person{}
	attendance := []Attendance{}
	// A gate account can look up a specific card, but cannot download the full directory/history.
	if actor.Role != "petugas" {
		rows, err := s.db.QueryContext(r.Context(), `SELECT id,token,name,number,role,group_name,active,demo FROM people ORDER BY name`)
		if err != nil {
			s.fail(w, err)
			return
		}
		for rows.Next() {
			var p Person
			var active, demo bool
			if err := rows.Scan(&p.ID, &p.Token, &p.Name, &p.Number, &p.Role, &p.GroupName, &active, &demo); err != nil {
				rows.Close()
				s.fail(w, err)
				return
			}
			p.Active = boolInt(active)
			p.Demo = boolInt(demo)
			people = append(people, p)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			s.fail(w, err)
			return
		}
		rows.Close()
		rows, err = s.db.QueryContext(r.Context(), `SELECT id,person_id,day::text,arrived,departed,status,late_minutes,note,source FROM attendance WHERE day BETWEEN $1 AND $2 ORDER BY day DESC, arrived DESC NULLS LAST, id`, from, to)
		if err != nil {
			s.fail(w, err)
			return
		}
		for rows.Next() {
			var a Attendance
			if err := rows.Scan(&a.ID, &a.PersonID, &a.Day, &a.Arrived, &a.Departed, &a.Status, &a.LateMinutes, &a.Note, &a.Source); err != nil {
				rows.Close()
				s.fail(w, err)
				return
			}
			attendance = append(attendance, a)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			s.fail(w, err)
			return
		}
		rows.Close()
	}
	writeJSON(w, 200, map[string]any{"people": people, "attendance": attendance, "settings": cfg, "today": day, "serverTime": s.now().UTC().Format(time.RFC3339Nano)})
}

func (s *Server) personAction(w http.ResponseWriter, r *http.Request, b actionBody) {
	name, number, group := strings.TrimSpace(b.Name), strings.TrimSpace(b.Number), strings.TrimSpace(b.Group)
	if name == "" || len(name) > 100 || number == "" || len(number) > 40 || group == "" || len(group) > 60 || (b.Role != "murid" && b.Role != "guru") {
		writeJSON(w, 400, map[string]string{"error": "Lengkapi nama, nomor induk, jenis, dan kelas/bagian dengan benar."})
		return
	}
	profile := studentProfile{}
	if b.Student != nil {
		if b.Role != "murid" {
			writeJSON(w, 400, map[string]string{"error": "Biodata siswa hanya berlaku untuk murid."})
			return
		}
		profile = *b.Student
		if message := profile.normalize(); message != "" {
			writeJSON(w, 400, map[string]string{"error": message})
			return
		}
	}
	// Keep identity and profile changes atomic. An omitted profile preserves existing data.
	if b.ID == "" {
		id, err := newID()
		if err != nil {
			s.fail(w, err)
			return
		}
		token, err := newID()
		if err != nil {
			s.fail(w, err)
			return
		}
		_, err = s.db.ExecContext(r.Context(), `INSERT INTO people(id, token, name, number, role, group_name,
			nisn, gender, class_parallel, birth_place, birth_date, nik, home_address)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,'')::date,$12,$13)`,
			id, token, name, number, b.Role, group, profile.NISN, profile.Gender, profile.ClassParallel,
			profile.BirthPlace, profile.BirthDate, profile.NIK, profile.HomeAddress)
		if err != nil {
			s.personWriteError(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"message": "Data baru dan kartu QR berhasil dibuat."})
		return
	}
	res, err := s.db.ExecContext(r.Context(), `UPDATE people SET name=$1, number=$2, role=$3, group_name=$4,
		nisn=CASE WHEN $6 THEN $7 ELSE nisn END,
		gender=CASE WHEN $6 THEN $8 ELSE gender END,
		class_parallel=CASE WHEN $6 THEN $9 ELSE class_parallel END,
		birth_place=CASE WHEN $6 THEN $10 ELSE birth_place END,
		birth_date=CASE WHEN $6 THEN NULLIF($11,'')::date ELSE birth_date END,
		nik=CASE WHEN $6 THEN $12 ELSE nik END,
		home_address=CASE WHEN $6 THEN $13 ELSE home_address END
		WHERE id=$5 AND demo=false`, name, number, b.Role, group, b.ID, b.Student != nil, profile.NISN,
		profile.Gender, profile.ClassParallel, profile.BirthPlace, profile.BirthDate, profile.NIK, profile.HomeAddress)
	if err != nil {
		s.personWriteError(w, err)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		var demo bool
		if s.db.QueryRowContext(r.Context(), `SELECT demo FROM people WHERE id=$1`, b.ID).Scan(&demo) == nil && demo {
			writeJSON(w, 409, map[string]string{"error": "Data contoh tidak dapat diubah. Hapus lewat Hapus data contoh."})
			return
		}
		writeJSON(w, 404, map[string]string{"error": "Data tidak ditemukan."})
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Data berhasil diperbarui."})
}

func (s *Server) personWriteError(w http.ResponseWriter, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "people_student_nisn_idx":
			writeJSON(w, 409, map[string]string{"error": "NISN sudah terdaftar."})
			return
		case "people_number_key":
			writeJSON(w, 409, map[string]string{"error": "Nomor induk sudah terdaftar."})
			return
		}
	}
	s.fail(w, err)
}

func (s *Server) activeAction(w http.ResponseWriter, r *http.Request, b actionBody) {
	if b.ID == "" || b.Active == nil {
		writeJSON(w, 400, map[string]string{"error": "ID dan status aktif berupa boolean diperlukan."})
		return
	}
	res, err := s.db.ExecContext(r.Context(), `UPDATE people SET active=$1 WHERE id=$2`, *b.Active, b.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "Data tidak ditemukan."})
		return
	}
	message := "Data dinonaktifkan. Riwayat absensi tetap tersimpan."
	if *b.Active {
		message = "Data diaktifkan."
	}
	writeJSON(w, 200, map[string]string{"message": message})
}

func findActivePerson(ctx context.Context, db *sql.DB, token string) (Person, error) {
	var p Person
	var active, demo bool
	err := db.QueryRowContext(ctx, `SELECT id,token,name,number,role,group_name,active,demo FROM people WHERE token=$1 AND active=true`, token).Scan(&p.ID, &p.Token, &p.Name, &p.Number, &p.Role, &p.GroupName, &active, &demo)
	p.Active = boolInt(active)
	p.Demo = boolInt(demo)
	return p, err
}

func findAttendance(ctx context.Context, db *sql.DB, id, day string) (*Attendance, error) {
	var a Attendance
	err := db.QueryRowContext(ctx, `SELECT id,person_id,day::text,arrived,departed,status,late_minutes,note,source FROM attendance WHERE person_id=$1 AND day=$2`, id, day).Scan(&a.ID, &a.PersonID, &a.Day, &a.Arrived, &a.Departed, &a.Status, &a.LateMinutes, &a.Note, &a.Source)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Server) scanAction(w http.ResponseWriter, r *http.Request, actor User, b actionBody) {
	if b.Action == "scan" && b.Mode != "masuk" && b.Mode != "pulang" {
		writeJSON(w, 400, map[string]string{"error": "Pilih absensi masuk atau pulang."})
		return
	}
	token := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(b.Token), "HADIR:"))
	p, err := findActivePerson(r.Context(), s.db, token)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, 404, map[string]string{"error": "QR tidak terdaftar atau kartu sudah nonaktif."})
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	cfg, err := readSettings(r.Context(), s.db)
	if err != nil {
		s.fail(w, err)
		return
	}
	now := s.now()
	day, minutes, err := dayAndMinutes(now, cfg.Timezone)
	if err != nil {
		s.fail(w, err)
		return
	}
	old, err := findAttendance(r.Context(), s.db, p.ID, day)
	if err != nil {
		s.fail(w, err)
		return
	}
	if b.Action == "lookup" {
		writeJSON(w, 200, map[string]any{"person": p, "record": old})
		return
	}
	if b.Mode == "pulang" {
		if old == nil || old.Arrived == nil {
			writeJSON(w, 400, map[string]any{"error": "Belum ada absensi masuk hari ini. Status izin/sakit/alpa tidak dapat dipulangkan.", "person": p})
			return
		}
		if old.Departed != nil {
			writeJSON(w, 409, map[string]any{"error": "Absensi pulang hari ini sudah tercatat.", "person": p})
			return
		}
		if now.Sub(*old.Arrived) < minPulangGap {
			writeJSON(w, 409, map[string]any{"error": "Baru masuk beberapa menit lalu. Tunggu sebelum mencatat pulang.", "person": p})
			return
		}
		var updated string
		err = s.db.QueryRowContext(r.Context(), `UPDATE attendance SET departed=$1 WHERE id=$2 AND departed IS NULL AND arrived IS NOT NULL RETURNING id`, now, old.ID).Scan(&updated)
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, 409, map[string]any{"error": "Absensi pulang hari ini sudah tercatat.", "person": p})
			return
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"message": "Absensi pulang berhasil dicatat.", "person": p})
		return
	}
	if old != nil && old.Arrived != nil {
		writeJSON(w, 409, map[string]any{"error": "Sudah absen masuk hari ini. Data tidak digandakan.", "person": p})
		return
	}
	cutoff := cfg.StudentTime
	if p.Role == "guru" {
		cutoff = cfg.TeacherTime
	}
	late := minutes - clockMinutes(cutoff)
	if late < 0 {
		late = 0
	}
	status := "Tepat waktu"
	if late > 0 {
		status = "Terlambat"
	}
	if old != nil {
		// Izin/Sakit/Alpa row (arrived NULL): person actually showed up, convert it.
		tx, err := s.db.BeginTx(r.Context(), nil)
		if err != nil {
			s.fail(w, err)
			return
		}
		defer tx.Rollback()
		after := *old
		after.Arrived, after.Status, after.LateMinutes, after.Source = &now, status, late, "scan"
		var updated string
		err = tx.QueryRowContext(r.Context(), `UPDATE attendance SET arrived=$1,status=$2,late_minutes=$3,source='scan' WHERE id=$4 AND arrived IS NULL RETURNING id`, now, status, late, old.ID).Scan(&updated)
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, 409, map[string]any{"error": "Sudah absen masuk hari ini. Data tidak digandakan.", "person": p})
			return
		}
		if err == nil {
			err = writeAudit(r.Context(), tx, after, actor, "correction", "Datang setelah dicatat "+old.Status, old)
		}
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			s.fail(w, err)
			return
		}
	} else {
		id, err := newID()
		if err != nil {
			s.fail(w, err)
			return
		}
		_, err = s.db.ExecContext(r.Context(), `INSERT INTO attendance(id,person_id,day,arrived,status,late_minutes) VALUES($1,$2,$3,$4,$5,$6)`, id, p.ID, day, now, status, late)
		if uniqueViolation(err) {
			writeJSON(w, 409, map[string]any{"error": "Absensi hari ini sudah tercatat.", "person": p})
			return
		}
		if err != nil {
			s.fail(w, err)
			return
		}
	}
	message := "Kehadiran tepat waktu berhasil dicatat."
	if late > 0 {
		message = fmt.Sprintf("Kehadiran tercatat · terlambat %d menit.", late)
	}
	writeJSON(w, 200, map[string]any{"message": message, "person": p})
}

func (s *Server) settingsAction(w http.ResponseWriter, r *http.Request, b actionBody) {
	cfg := b.Settings
	cfg.School = strings.TrimSpace(cfg.School)
	if cfg.School == "" || len(cfg.School) > 100 || !validClock(cfg.StudentTime) || !validClock(cfg.TeacherTime) || !validClock(cfg.StudentDepartureTime) || !validClock(cfg.TeacherDepartureTime) || (cfg.Timezone != "Asia/Jakarta" && cfg.Timezone != "Asia/Makassar" && cfg.Timezone != "Asia/Jayapura") {
		writeJSON(w, 400, map[string]string{"error": "Periksa nama sekolah, jam masuk/pulang, dan zona waktu."})
		return
	}
	if old, err := readSettings(r.Context(), s.db); err == nil && old.Timezone != cfg.Timezone {
		today, _, err := dayAndMinutes(s.now(), old.Timezone)
		var n int
		if err == nil {
			err = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM attendance WHERE day=$1`, today).Scan(&n)
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		if n > 0 {
			writeJSON(w, 409, map[string]string{"error": "Zona waktu tidak dapat diubah karena hari ini sudah ada catatan absensi."})
			return
		}
	}
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO settings(id,school,student_time,teacher_time,student_departure_time,teacher_departure_time,timezone) VALUES('main',$1,$2,$3,$4,$5,$6) ON CONFLICT(id) DO UPDATE SET school=excluded.school,student_time=excluded.student_time,teacher_time=excluded.teacher_time,student_departure_time=excluded.student_departure_time,teacher_departure_time=excluded.teacher_departure_time,timezone=excluded.timezone`, cfg.School, cfg.StudentTime, cfg.TeacherTime, cfg.StudentDepartureTime, cfg.TeacherDepartureTime, cfg.Timezone)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Pengaturan berhasil disimpan."})
}

var demoNames = []string{"Aditya Pratama", "Alya Putri", "Bima Ramadhan", "Citra Maharani", "Daffa Saputra", "Fadila Zahra", "Gilang Nugraha", "Hana Azzahra", "Irfan Maulana", "Kirana Putri", "Luthfi Hakim", "Nadia Safira", "Rina Wulandari", "Ahmad Fauzi", "Dewi Lestari"}

func (s *Server) demoAction(w http.ResponseWriter, r *http.Request) {
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback()
	groups := []string{"VII A", "VII B", "VIII A", "IX A"}
	teacherGroups := []string{"Matematika", "Bahasa Indonesia", "IPA"}
	for i, name := range demoNames {
		token, err := newID()
		if err != nil {
			s.fail(w, err)
			return
		}
		role := "murid"
		group := groups[i%4]
		if i >= 12 {
			role = "guru"
			group = teacherGroups[i-12]
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO people(id,token,name,number,role,group_name,demo) VALUES($1,$2,$3,$4,$5,$6,true) ON CONFLICT DO NOTHING`, fmt.Sprintf("demo-%d", i), token, name, fmt.Sprintf("CONTOH-%d", i+1), role, group)
		if err != nil {
			s.fail(w, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"message": "15 data contoh ditambahkan. Belum ada absensi yang dibuat."})
}

func (s *Server) clearDemoAction(w http.ResponseWriter, r *http.Request) {
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM attendance_audit WHERE person_id IN (SELECT id FROM people WHERE demo=true)`); err != nil {
		s.fail(w, err)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM attendance WHERE person_id IN (SELECT id FROM people WHERE demo=true)`); err != nil {
		s.fail(w, err)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM people WHERE demo=true`); err != nil {
		s.fail(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Data contoh beserta absensi contohnya dihapus."})
}
