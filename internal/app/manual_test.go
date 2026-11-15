package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestManualStatusAndAudit(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to disposable hadir_stage1_test")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Path != "/hadir_stage1_test" {
		t.Fatal("refusing to modify a non-test database")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`TRUNCATE attendance_audit,attendance,sessions,users,people,settings RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if err := CreateAdmin(context.Background(), db, "admin3@example.test", "a strong test password"); err != nil {
		t.Fatal(err)
	}
	s := NewServer(db, false)
	s.now = func() time.Time { return time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC) }
	call := func(method, path string, body any, cookie *http.Cookie) (int, map[string]any, *http.Cookie) {
		t.Helper()
		var payload []byte
		if body != nil {
			payload, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(payload))
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode %s %s: %v: %s", method, path, err, w.Body.String())
		}
		var set *http.Cookie
		for _, c := range w.Result().Cookies() {
			if c.Name == cookieName {
				set = c
			}
		}
		return w.Code, out, set
	}
	code, _, admin := call("POST", "/api/auth/login", map[string]any{"email": "admin3@example.test", "password": "a strong test password"}, nil)
	if code != 200 || admin == nil {
		t.Fatalf("admin login %d", code)
	}
	for _, item := range []map[string]string{
		{"email": "gate3@example.test", "name": "Gerbang", "role": "petugas", "password": "a gate password 123"},
		{"email": "operator3@example.test", "name": "Operator", "role": "operator", "password": "an operator password 123"},
	} {
		body := map[string]any{"action": "create", "email": item["email"], "name": item["name"], "role": item["role"], "password": item["password"]}
		if code, _, _ := call("POST", "/api/users", body, admin); code != 201 {
			t.Fatalf("create %s: %d", item["role"], code)
		}
	}
	code, _, gate := call("POST", "/api/auth/login", map[string]any{"email": "gate3@example.test", "password": "a gate password 123"}, nil)
	if code != 200 || gate == nil {
		t.Fatalf("gate login %d", code)
	}
	code, _, operator := call("POST", "/api/auth/login", map[string]any{"email": "operator3@example.test", "password": "an operator password 123"}, nil)
	if code != 200 || operator == nil {
		t.Fatalf("operator login %d", code)
	}
	code, got, _ := call("GET", "/api/hadir", nil, admin)
	if code != 200 || got["settings"].(map[string]any)["student_departure_time"] != "13:00" {
		t.Fatalf("default departure time: %d %#v", code, got["settings"])
	}
	settings := map[string]any{"action": "settings", "school": "MTs Amal Bakti", "student_time": "07:00", "teacher_time": "06:45", "student_departure_time": "13:15", "teacher_departure_time": "14:30", "timezone": "Asia/Jakarta"}
	if code, _, _ := call("POST", "/api/hadir", settings, operator); code != 403 {
		t.Fatalf("operator changed settings: %d", code)
	}
	settings["student_departure_time"] = "25:00"
	if code, _, _ := call("POST", "/api/hadir", settings, admin); code != 400 {
		t.Fatalf("invalid departure accepted: %d", code)
	}
	settings["student_departure_time"] = "13:15"
	if code, _, _ := call("POST", "/api/hadir", settings, admin); code != 200 {
		t.Fatalf("save departure settings: %d", code)
	}
	for _, number := range []string{"30001", "30002"} {
		if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "person", "name": "Siswa " + number, "number": number, "role": "murid", "group": "VII A"}, admin); code != 200 {
			t.Fatalf("person %s: %d", number, code)
		}
	}
	code, got, _ = call("POST", "/api/hadir", map[string]any{"action": "lookupNumber", "number": "30001"}, gate)
	if code != 200 || got["person"].(map[string]any)["token"] != nil {
		t.Fatalf("number lookup leaked token: %d %#v", code, got)
	}
	personID := got["person"].(map[string]any)["id"].(string)
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "lookupNumber", "number": "unknown"}, gate); code != 404 {
		t.Fatalf("unknown number: %d", code)
	}
	manual := map[string]any{"action": "manual", "person_id": personID, "day": "2026-11-15", "status": "Sakit", "reason": "Surat dokter"}
	if code, _, _ := call("POST", "/api/hadir", manual, gate); code != 201 {
		t.Fatalf("manual sick: %d", code)
	}
	if code, _, _ := call("POST", "/api/hadir", manual, gate); code != 409 {
		t.Fatalf("duplicate manual: %d", code)
	}
	manual["day"] = "2026-11-14"
	if code, _, _ := call("POST", "/api/hadir", manual, gate); code != 400 {
		t.Fatalf("gate backdated status: %d", code)
	}
	manual["day"] = "2026-11-16"
	if code, _, _ := call("POST", "/api/hadir", manual, admin); code != 400 {
		t.Fatalf("future status: %d", code)
	}
	manual["day"] = "2026-11-15"
	manual["status"] = "Hadir"
	manual["arrived"] = "2026-11-15T00:30:00Z"
	if code, _, _ := call("POST", "/api/hadir", manual, gate); code != 403 {
		t.Fatalf("gate manual present: %d", code)
	}
	code, got, _ = call("GET", "/api/hadir", nil, admin)
	record := got["attendance"].([]any)[0].(map[string]any)
	if code != 200 || record["status"] != "Sakit" || record["arrived"] != nil || record["note"] != "Surat dokter" {
		t.Fatalf("manual absence data: %d %#v", code, record)
	}
	recordID := record["id"].(string)
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "correct", "id": recordID, "status": "Sakit", "note": "Surat dokter", "reason": "Tidak ada perubahan"}, admin); code != 400 {
		t.Fatalf("no-op from manual source: %d", code)
	}
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "correct", "id": recordID, "status": "Hadir", "arrived": "2026-11-15T00:30:00Z", "reason": "Koreksi masuk"}, operator); code != 403 {
		t.Fatalf("operator corrected: %d", code)
	}
	if code, _, _ := call("GET", "/api/audit", nil, gate); code != 403 {
		t.Fatalf("gate audit access: %d", code)
	}
	if code, _, _ := call("GET", "/api/audit", nil, operator); code != 403 {
		t.Fatalf("operator audit access: %d", code)
	}
	correction := map[string]any{"action": "correct", "id": recordID, "status": "Hadir", "arrived": "2026-11-15T00:30:00Z", "departed": "2026-11-15T06:10:00Z", "note": "Sudah diverifikasi", "reason": "Koreksi hasil verifikasi"}
	if code, _, _ := call("POST", "/api/hadir", correction, admin); code != 200 {
		t.Fatalf("correct sick to present: %d", code)
	}
	code, got, _ = call("GET", "/api/hadir", nil, admin)
	record = got["attendance"].([]any)[0].(map[string]any)
	if code != 200 || record["status"] != "Terlambat" || record["late_minutes"] != float64(30) || record["departed"] == nil {
		t.Fatalf("corrected present values: %d %#v", code, record)
	}
	code, got, _ = call("GET", "/api/audit?attendance_id="+recordID, nil, admin)
	audit := got["audit"].([]any)
	if code != 200 || len(audit) != 2 {
		t.Fatalf("audit count: %d %#v", code, got)
	}
	latest := audit[0].(map[string]any)
	if latest["action"] != "correction" || latest["reason"] != "Koreksi hasil verifikasi" || latest["before"].(map[string]any)["status"] != "Sakit" || latest["after"].(map[string]any)["status"] != "Terlambat" {
		t.Fatalf("audit before/after: %#v", latest)
	}
	correction = map[string]any{"action": "correct", "id": recordID, "status": "Izin", "note": "Kegiatan keluarga", "reason": "Dokumen izin diterima"}
	if code, _, _ := call("POST", "/api/hadir", correction, admin); code != 200 {
		t.Fatalf("correct present to absence: %d", code)
	}
	if code, _, _ := call("POST", "/api/hadir", correction, admin); code != 400 {
		t.Fatalf("no-op correction: %d", code)
	}
	code, got, _ = call("GET", "/api/hadir", nil, admin)
	record = got["attendance"].([]any)[0].(map[string]any)
	if code != 200 || record["status"] != "Izin" || record["arrived"] != nil || record["departed"] != nil || record["late_minutes"] != float64(0) {
		t.Fatalf("corrected absence values: %d %#v", code, record)
	}
	code, got, _ = call("GET", "/api/hadir", nil, admin)
	var secondID string
	for _, item := range got["people"].([]any) {
		p := item.(map[string]any)
		if p["number"] == "30002" {
			secondID = p["id"].(string)
		}
	}
	if secondID == "" {
		t.Fatal("second person missing")
	}
	backfill := map[string]any{"action": "manual", "person_id": secondID, "day": "2026-11-14", "status": "Hadir", "arrived": "2026-11-14T00:00:00Z", "reason": "Kartu tidak terbaca"}
	if code, _, _ := call("POST", "/api/hadir", backfill, admin); code != 201 {
		t.Fatalf("admin backfill present: %d", code)
	}
	code, got, _ = call("GET", "/api/audit", nil, admin)
	if code != 200 || len(got["audit"].([]any)) != 4 {
		t.Fatalf("audit count after backfill: %d %#v", code, got)
	}
}
