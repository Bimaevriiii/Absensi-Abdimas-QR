package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestStudentDirectoryUpgrade(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to disposable hadir_stage1_test")
	}
	dbURL, err := url.Parse(dsn)
	if err != nil || dbURL.Path != "/hadir_stage1_test" {
		t.Fatal("refusing to modify a non-test database")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema, err := newID()
	if err != nil {
		t.Fatal(err)
	}
	schema = "student_test_" + strings.ReplaceAll(schema, "-", "")
	if _, err := db.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Error(err)
			return
		}
		defer cleanup.Close()
		if _, err := cleanup.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error(err)
		}
	})

	params := dbURL.Query()
	params.Set("search_path", schema)
	dbURL.RawQuery = params.Encode()
	testDB, err := sql.Open("pgx", dbURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer testDB.Close()

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := testDB.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}

	exec(`CREATE TABLE schema_migrations (name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`)
	for _, name := range []string{"0001_initial.sql", "0002_manual_status_audit.sql", "0003_attendance_day_index.sql"} {
		body, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		exec(string(body))
		exec("INSERT INTO schema_migrations(name) VALUES ($1)", name)
	}
	exec(`INSERT INTO people(id,token,name,number,role,group_name) VALUES
		('student-old','qr-old','Siswa Lama','0000000007','murid','VII A'),
		('teacher-old','qr-teacher','Guru Lama','NIP-001','guru','IPA')`)
	exec(`INSERT INTO attendance(id,person_id,day,arrived,status) VALUES
		('record-old','student-old','2026-11-20','2026-11-19T23:58:00Z','Tepat waktu')`)

	server := NewServer(testDB, false)
	server.now = func() time.Time { return time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC) }
	sessions := map[string]*http.Cookie{}
	for _, role := range []string{"pengelola", "operator", "petugas"} {
		token, err := newSecret()
		if err != nil {
			t.Fatal(err)
		}
		exec(`INSERT INTO users(id,email,name,role,password_hash) VALUES ($1,$2,$1,$1,'unused')`, role, role+"@example.test")
		exec(`INSERT INTO sessions(token_hash,user_id,expires_at) VALUES ($1,$2,$3)`, tokenHash(token), role, server.now().Add(time.Hour))
		sessions[role] = &http.Cookie{Name: cookieName, Value: token}
	}
	call := func(role, method, path, payload string, expected int, fragments ...string) string {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(payload))
		if method == "POST" {
			request.Header.Set("Content-Type", "application/json")
		}
		if cookie := sessions[role]; cookie != nil {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != expected || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s %s (%s): status %d, want %d: %s", method, path, role, response.Code, expected, response.Body.String())
		}
		for _, fragment := range fragments {
			if !strings.Contains(response.Body.String(), fragment) {
				t.Fatalf("%s %s missing %q: %s", method, path, fragment, response.Body.String())
			}
		}
		return response.Body.String()
	}

	reportBefore := call("pengelola", "GET", "/api/hadir", "", 200, "record-old", "qr-old", "VII A", "Guru Lama")
	for range 2 {
		if err := Migrate(context.Background(), testDB); err != nil {
			t.Fatal(err)
		}
	}
	reportAfter := call("pengelola", "GET", "/api/hadir", "", 200)
	if reportAfter != reportBefore {
		t.Fatal("migrating the directory changed the attendance response")
	}
	call("pengelola", "GET", "/api/students", "", 200, `"id":"student-old"`, `"birth_date":""`)
	call("", "GET", "/api/students", "", 401)
	call("petugas", "GET", "/api/students", "", 403)

	profile := studentProfile{
		NISN: "0000000007", Gender: "P", ClassParallel: "3", BirthPlace: "Kota Uji",
		BirthDate: "2012-01-15", NIK: "0000000000000098", HomeAddress: "Alamat sintetis",
	}
	writePerson := func(role, id, name, number string, profile *studentProfile, status int, fragments ...string) {
		t.Helper()
		body, err := json.Marshal(actionBody{
			Action: "person", ID: id, Name: name, Number: number, Role: "murid", Group: "9", Student: profile,
		})
		if err != nil {
			t.Fatal(err)
		}
		call(role, "POST", "/api/hadir", string(body), status, fragments...)
	}
	writePerson("petugas", "student-old", "Siswa Lama", "0000000007", &profile, 403)
	writePerson("operator", "", "Siswa Ringkas", "0000000018", nil, 200)
	writePerson("operator", "", "Siswa Lengkap", "000000000000000031", &profile, 200)
	call("operator", "GET", "/api/students", "", 200, profile.NIK, profile.HomeAddress)
	writePerson("operator", "student-old", "Nama Harus Tetap", "0000000007", &profile, 409, "NISN sudah terdaftar")
	call("pengelola", "GET", "/api/hadir", "", 200, "Siswa Lama", "VII A", "record-old", "000000000000000031")
	for _, invalid := range []studentProfile{
		{NISN: "123"}, {NIK: "123456789012345G"}, {BirthDate: "2012-02-30"},
	} {
		writePerson("operator", "student-old", "Nama Harus Tetap", "0000000007", &invalid, 400)
	}

	lookup := func(number, kind string, status int, fragments ...string) string {
		t.Helper()
		body, _ := json.Marshal(actionBody{Action: "lookupNumber", Number: number, NumberType: kind})
		return call("petugas", "POST", "/api/hadir", string(body), status, fragments...)
	}
	ambiguity := lookup("0000000007", "", 409, "ambiguous_number")
	if strings.Contains(ambiguity, "Siswa") {
		t.Fatal("ambiguous lookup exposed an identity")
	}
	lookup("0000000007", "number", 200, "Siswa Lama")
	identity := lookup("0000000007", "nisn", 200, "Siswa Lengkap", "000000000000000031")
	lookup("000000000000000031", "", 200, "Siswa Lengkap")
	lookup("NIP-001", "", 200, "Guru Lama")
	writePerson("pengelola", "student-old", "Siswa Lama", "0000000098", nil, 200)
	writePerson("pengelola", "student-old", "Siswa Lama", "0000000098", &studentProfile{NISN: "0000000098", Gender: "L"}, 200)
	lookup("0000000098", "", 200, "Siswa Lama")
	writePerson("pengelola", "student-old", "Siswa Lama Diubah", "0000000098", nil, 200)
	call("operator", "GET", "/api/students", "", 200, `"nisn":"0000000098"`, `"gender":"L"`)

	for _, response := range []string{
		identity,
		call("operator", "GET", "/api/hadir", "", 200, "record-old"),
		call("petugas", "POST", "/api/hadir", `{"action":"lookup","token":"qr-old"}`, 200, "Siswa Lama Diubah"),
	} {
		for _, key := range []string{`"nisn"`, `"nik"`, `"home_address"`, `"birth_date"`, `"birth_place"`} {
			if strings.Contains(response, key) {
				t.Fatalf("attendance response exposed %s", key)
			}
		}
	}
}
