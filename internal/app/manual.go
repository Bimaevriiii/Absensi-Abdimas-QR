package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

func absenceStatus(status string) bool {
	return status == "Izin" || status == "Sakit" || status == "Alpa"
}

func validReason(reason string) bool {
	return len([]rune(reason)) >= 5 && len([]rune(reason)) <= 500
}

func validAttendanceDay(day, today string) bool {
	parsed, err := time.Parse("2006-01-02", day)
	return err == nil && parsed.Format("2006-01-02") == day && day >= "2000-01-01" && day <= today
}

func (s *Server) lookupNumberAction(w http.ResponseWriter, r *http.Request, b actionBody) {
	number := strings.TrimSpace(b.Number)
	if number == "" || len(number) > 40 {
		writeJSON(w, 400, map[string]string{"error": "Masukkan nomor induk yang tepat."})
		return
	}
	if b.NumberType != "" && b.NumberType != "number" && b.NumberType != "nisn" {
		writeJSON(w, 400, map[string]string{"error": "Jenis nomor tidak valid."})
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, name, number, role, group_name FROM people
		WHERE active=true AND (($2 <> 'nisn' AND number=$1)
		OR ($2 <> 'number' AND role='murid' AND nisn <> '' AND nisn=$1)) LIMIT 2`, number, b.NumberType)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer rows.Close()
	var id, name, localNumber, role, group string
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, 404, map[string]string{"error": "Nomor induk tidak ditemukan atau peserta nonaktif."})
		return
	}
	if err := rows.Scan(&id, &name, &localNumber, &role, &group); err != nil {
		s.fail(w, err)
		return
	}
	if rows.Next() {
		writeJSON(w, 409, map[string]string{"code": "ambiguous_number", "error": "Nomor cocok dengan dua orang. Pilih NIS Lokal / NIP atau NISN."})
		return
	}
	if err := rows.Err(); err != nil {
		s.fail(w, err)
		return
	}
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
	record, err := findAttendance(r.Context(), s.db, id, day)
	if err != nil {
		s.fail(w, err)
		return
	}
	// Exact-number lookup returns only one identity and never exposes its QR token.
	writeJSON(w, 200, map[string]any{"person": map[string]string{"id": id, "name": name, "number": localNumber, "role": role, "group_name": group}, "record": record})
}

func parsePresent(b actionBody, day, timezone, role string, cfg Settings, now time.Time) (*time.Time, *time.Time, string, int, error) {
	arrived, err := time.Parse(time.RFC3339Nano, b.Arrived)
	if err != nil {
		return nil, nil, "", 0, errors.New("Waktu masuk harus berformat RFC3339")
	}
	localDay, minutes, err := dayAndMinutes(arrived, timezone)
	if err != nil || localDay != day || arrived.After(now.Add(5*time.Minute)) {
		return nil, nil, "", 0, errors.New("Waktu masuk harus berada pada tanggal sekolah yang dipilih dan tidak di masa depan")
	}
	var departed *time.Time
	if b.Departed != "" {
		out, err := time.Parse(time.RFC3339Nano, b.Departed)
		if err != nil || out.Before(arrived) || out.Sub(arrived) > 24*time.Hour || out.After(now.Add(5*time.Minute)) {
			return nil, nil, "", 0, errors.New("Waktu pulang harus setelah masuk, maksimal 24 jam, dan tidak di masa depan")
		}
		departed = &out
	}
	cutoff := cfg.StudentTime
	if role == "guru" {
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
	return &arrived, departed, status, late, nil
}

func writeAudit(ctx context.Context, tx *sql.Tx, record Attendance, actor User, action, reason string, before *Attendance) error {
	id, err := newID()
	if err != nil {
		return err
	}
	afterJSON, err := json.Marshal(record)
	if err != nil {
		return err
	}
	var beforeJSON any
	if before != nil {
		raw, err := json.Marshal(before)
		if err != nil {
			return err
		}
		beforeJSON = string(raw)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO attendance_audit(id,attendance_id,person_id,actor_user_id,action,before_data,after_data,reason) VALUES($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb,$8)`, id, record.ID, record.PersonID, actor.ID, action, beforeJSON, string(afterJSON), reason)
	return err
}

func (s *Server) manualAction(w http.ResponseWriter, r *http.Request, actor User, b actionBody) {
	cfg, err := readSettings(r.Context(), s.db)
	if err != nil {
		s.fail(w, err)
		return
	}
	today, _, err := dayAndMinutes(s.now(), cfg.Timezone)
	if err != nil {
		s.fail(w, err)
		return
	}
	day := b.Day
	if day == "" {
		day = today
	}
	reason := strings.TrimSpace(b.Reason)
	note := strings.TrimSpace(b.Note)
	if !validAttendanceDay(day, today) || (actor.Role == "petugas" && day != today) || b.PersonID == "" || !validReason(reason) || len([]rune(note)) > 500 || (!absenceStatus(b.Status) && b.Status != "Hadir") {
		writeJSON(w, 400, map[string]string{"error": "Periksa peserta, tanggal, status, dan alasan minimal 5 karakter."})
		return
	}
	if b.Status == "Hadir" && actor.Role != "pengelola" {
		writeJSON(w, 403, map[string]string{"error": "Hanya pengelola dapat menambah catatan hadir yang terlewat."})
		return
	}
	var role string
	err = s.db.QueryRowContext(r.Context(), `SELECT role FROM people WHERE id=$1 AND active=true`, b.PersonID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, 404, map[string]string{"error": "Peserta tidak ditemukan atau nonaktif."})
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	id, err := newID()
	if err != nil {
		s.fail(w, err)
		return
	}
	record := Attendance{ID: id, PersonID: b.PersonID, Day: day, Status: b.Status, Note: note, Source: "manual"}
	if b.Status == "Hadir" {
		arrived, departed, status, late, err := parsePresent(b, day, cfg.Timezone, role, cfg, s.now())
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		record.Arrived, record.Departed, record.Status, record.LateMinutes = arrived, departed, status, late
	} else if b.Arrived != "" || b.Departed != "" {
		writeJSON(w, 400, map[string]string{"error": "Status izin/sakit/alpa tidak mempunyai jam masuk atau pulang."})
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO attendance(id,person_id,day,arrived,departed,status,late_minutes,note,source) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, record.ID, record.PersonID, record.Day, record.Arrived, record.Departed, record.Status, record.LateMinutes, record.Note, record.Source)
	if uniqueViolation(err) {
		writeJSON(w, 409, map[string]string{"error": "Peserta sudah memiliki catatan pada tanggal tersebut. Gunakan koreksi oleh pengelola."})
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := writeAudit(r.Context(), tx, record, actor, "manual_create", reason, nil); err != nil {
		s.fail(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"message": "Status manual berhasil dicatat.", "record": record})
}

func (s *Server) correctAction(w http.ResponseWriter, r *http.Request, actor User, b actionBody) {
	reason := strings.TrimSpace(b.Reason)
	note := strings.TrimSpace(b.Note)
	if b.ID == "" || !validReason(reason) || len([]rune(note)) > 500 || (!absenceStatus(b.Status) && b.Status != "Hadir") {
		writeJSON(w, 400, map[string]string{"error": "Pilih catatan, status, dan alasan koreksi minimal 5 karakter."})
		return
	}
	cfg, err := readSettings(r.Context(), s.db)
	if err != nil {
		s.fail(w, err)
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback()
	var before Attendance
	var role string
	err = tx.QueryRowContext(r.Context(), `SELECT a.id,a.person_id,a.day::text,a.arrived,a.departed,a.status,a.late_minutes,a.note,a.source,p.role FROM attendance a JOIN people p ON p.id=a.person_id WHERE a.id=$1 FOR UPDATE OF a`, b.ID).Scan(&before.ID, &before.PersonID, &before.Day, &before.Arrived, &before.Departed, &before.Status, &before.LateMinutes, &before.Note, &before.Source, &role)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, 404, map[string]string{"error": "Catatan absensi tidak ditemukan."})
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	after := before
	after.Note = note
	if b.Status == "Hadir" {
		arrived, departed, status, late, err := parsePresent(b, before.Day, cfg.Timezone, role, cfg, s.now())
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		after.Arrived, after.Departed, after.Status, after.LateMinutes = arrived, departed, status, late
	} else {
		if b.Arrived != "" || b.Departed != "" {
			writeJSON(w, 400, map[string]string{"error": "Status izin/sakit/alpa tidak mempunyai jam masuk atau pulang."})
			return
		}
		after.Status = b.Status
		after.Arrived, after.Departed = nil, nil
		after.LateMinutes = 0
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if string(beforeJSON) == string(afterJSON) {
		writeJSON(w, 400, map[string]string{"error": "Tidak ada perubahan pada catatan."})
		return
	}
	after.Source = "correction"
	_, err = tx.ExecContext(r.Context(), `UPDATE attendance SET arrived=$1,departed=$2,status=$3,late_minutes=$4,note=$5,source='correction' WHERE id=$6`, after.Arrived, after.Departed, after.Status, after.LateMinutes, after.Note, after.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := writeAudit(r.Context(), tx, after, actor, "correction", reason, &before); err != nil {
		s.fail(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"message": "Catatan berhasil dikoreksi dan riwayat perubahan disimpan.", "record": after})
}

func (s *Server) auditGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireUser(w, r, "pengelola"); !ok {
		return
	}
	recordID := strings.TrimSpace(r.URL.Query().Get("attendance_id"))
	if len(recordID) > 64 {
		writeJSON(w, 400, map[string]string{"error": "ID catatan tidak valid."})
		return
	}
	before := time.Now().Add(time.Hour)
	if v := r.URL.Query().Get("before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "Parameter before harus berformat RFC3339."})
			return
		}
		before = t
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT a.id,a.attendance_id,a.person_id,a.actor_user_id,u.name,a.action,a.before_data,a.after_data,a.reason,a.created_at FROM attendance_audit a JOIN users u ON u.id=a.actor_user_id WHERE ($1='' OR a.attendance_id=$1) AND a.created_at < $2 ORDER BY a.created_at DESC,a.id DESC LIMIT 100`, recordID, before)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, attendanceID, personID, actorID, actorName, action, reason string
		var beforeData, afterData []byte
		var createdAt time.Time
		if err := rows.Scan(&id, &attendanceID, &personID, &actorID, &actorName, &action, &beforeData, &afterData, &reason, &createdAt); err != nil {
			s.fail(w, err)
			return
		}
		out = append(out, map[string]any{"id": id, "attendance_id": attendanceID, "person_id": personID, "actor_user_id": actorID, "actor_name": actorName, "action": action, "before": json.RawMessage(beforeData), "after": json.RawMessage(afterData), "reason": reason, "created_at": createdAt})
	}
	if err := rows.Err(); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"audit": out})
}
