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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func importWorkbook(t *testing.T, rows ...[]any) []byte {
	t.Helper()
	template, err := studentTemplate()
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(template)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	for i, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, i+2)
		if err != nil {
			t.Fatal(err)
		}
		if err := book.SetSheetRow(studentSheet, cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	buf, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestStudentWorkbook(t *testing.T) {
	template, err := studentTemplate()
	if err != nil {
		t.Fatal(err)
	}
	data := template.Bytes()
	book, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	if !slices.Equal(book.GetSheetList(), []string{"Murid", "Contoh"}) {
		t.Fatalf("sheet list: %v", book.GetSheetList())
	}
	for _, col := range []string{"A", "B", "H"} {
		styleID, err := book.GetColStyle(studentSheet, col)
		if err != nil {
			t.Fatal(err)
		}
		style, err := book.GetStyle(styleID)
		if err != nil || style.NumFmt != 49 {
			t.Fatalf("expected text format in %s: %v", col, err)
		}
	}
	if _, _, err := readStudentWorkbook(bytes.NewReader(data)); err == nil || !strings.Contains(err.Error(), "belum berisi data") {
		t.Fatalf("blank student sheet should not import examples: %v", err)
	}

	input := importWorkbook(t,
		[]any{"000000000000000018", "0000000007", "Alya Uji", "P", 9, 3, "Bogor, 15 Januari 2012", "0000000000000016", "Alamat sintetis"},
		nil,
		[]any{"000000000000000018", "0000000007", "Duplikat", "", "9"},
		[]any{"002", "", "Uji Tanggal", "", "9", "", "Bogor, 30 Februari 2012"},
		[]any{"003", "", "NIK Angka", "", "9", "", "", int64(1234567890123456)},
	)
	students, issues, err := readStudentWorkbook(bytes.NewReader(input))
	if err != nil || len(students) != 1 || len(issues) != 4 {
		t.Fatalf("unexpected parse result: %d rows, %+v, %v", len(students), issues, err)
	}
	row := students[0]
	if row.number != "000000000000000018" || row.profile.NISN != "0000000007" || row.profile.NIK != "0000000000000016" ||
		row.group != "9" || row.profile.ClassParallel != "3" || row.profile.BirthDate != "2012-01-15" || row.profile.BirthPlace != "Bogor" {
		t.Fatalf("unexpected imported values: %+v", row)
	}
	if issues[0].Row != 4 || issues[1].Column != "NISN" || issues[2].Column != "TEMPAT/TANGGAL LAHIR" || issues[3].Column != "NIK SISWA" {
		t.Fatalf("unexpected issue positions: %+v", issues)
	}
}

func TestStudentImportTransactions(t *testing.T) {
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
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`TRUNCATE people, attendance, attendance_audit, users, sessions RESTART IDENTITY CASCADE`)
	exec(`INSERT INTO people(id,token,name,number,role,group_name,nisn,active) VALUES
		('old','qr-old','Nama Lama','010','murid','VII A','0000000010',false),
		('other','qr-other','Murid Lain','011','murid','VIII','0000000011',true),
		('teacher','qr-teacher','Guru','NIP-01','guru','IPA','',true)`)
	cookies := map[string]*http.Cookie{}
	for _, role := range []string{"pengelola", "operator", "petugas"} {
		token, err := newSecret()
		if err != nil {
			t.Fatal(err)
		}
		exec(`INSERT INTO users(id,email,name,role,password_hash) VALUES ($1,$2,$1,$1,'unused')`, role, role+"@example.test")
		exec(`INSERT INTO sessions(token_hash,user_id,expires_at) VALUES ($1,$2,$3)`, tokenHash(token), role, time.Now().Add(time.Hour))
		cookies[role] = &http.Cookie{Name: cookieName, Value: token}
	}

	server := NewServer(db, false)
	call := func(role, mode, origin string, data []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/students/import?mode="+mode, bytes.NewReader(data))
		r.Header.Set("Content-Type", xlsxMediaType)
		r.Header.Set("Origin", origin)
		if cookies[role] != nil {
			r.AddCookie(cookies[role])
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w
	}
	check := func(w *httptest.ResponseRecorder, expected int) studentImportResult {
		t.Helper()
		if w.Code != expected || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("expected HTTP %d, got %d: %s", expected, w.Code, w.Body.String())
		}
		var result studentImportResult
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	count := func(want int) {
		t.Helper()
		var n int
		if err := db.QueryRow("SELECT count(*) FROM people").Scan(&n); err != nil || n != want {
			t.Fatalf("expected %d people, got %d: %v", want, n, err)
		}
	}

	mixed := importWorkbook(t,
		[]any{"010", "0000000010", "Jangan Ubah", "P", "9"},
		[]any{"020", "0000000011", "Duplikat NISN", "", "9"},
		[]any{"000000000000000018", "", "Murid Baru Excel", "", "9"},
	)
	for _, mode := range []string{"preview", "commit"} {
		check(call("", mode, "", mixed), 401)
		check(call("petugas", mode, "", mixed), 403)
		check(call("operator", mode, "https://example.test", mixed), 403)
	}
	check(call("operator", "preview", "", []byte("not a workbook")), 422)
	check(call("operator", "preview", "", make([]byte, maxStudentUpload+1)), 413)

	preview := check(call("operator", "preview", "", mixed), 200)
	if preview.New != 1 || len(preview.Skipped) != 2 || len(preview.Errors) != 0 {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	count(3)
	commit := check(call("operator", "commit", "", mixed), 200)
	if commit.Added != 1 || len(commit.Skipped) != 2 {
		t.Fatalf("unexpected commit: %+v", commit)
	}
	retry := check(call("operator", "commit", "", mixed), 200)
	if retry.Added != 0 || len(retry.Skipped) != 3 {
		t.Fatalf("unexpected retry: %+v", retry)
	}
	count(4)
	var name, token string
	var active bool
	if err := db.QueryRow("SELECT name, token, active FROM people WHERE id='old'").Scan(&name, &token, &active); err != nil ||
		name != "Nama Lama" || token != "qr-old" || active {
		t.Fatalf("the import changed an existing inactive student: %v", err)
	}
	if err := db.QueryRow("SELECT token FROM people WHERE number='000000000000000018'").Scan(&token); err != nil || token == "" {
		t.Fatalf("new student has no token: %v", err)
	}

	invalid := importWorkbook(t,
		[]any{"021", "", "Valid", "", "9"},
		[]any{"010", "0000000011", "Konflik Dua Murid", "", "9"},
		[]any{"NIP-01", "", "Konflik Guru", "", "9"},
		[]any{"022", "", "Baris Akhir", "", ""},
	)
	rejected := check(call("pengelola", "commit", "", invalid), 422)
	if len(rejected.Errors) != 3 || rejected.Errors[2].Row != 5 || rejected.Errors[2].Column != "KLS" {
		t.Fatalf("unexpected errors: %+v", rejected.Errors)
	}
	count(4)

	exec(`ALTER TABLE people ADD CONSTRAINT test_student_import_reject CHECK (name <> 'Gagal saat insert')`)
	defer exec(`ALTER TABLE people DROP CONSTRAINT test_student_import_reject`)
	dbFailure := importWorkbook(t, []any{"021", "", "Valid", "", "9"}, []any{"022", "", "Gagal saat insert", "", "9"})
	check(call("pengelola", "commit", "", dbFailure), 500)
	count(4)

	concurrent := importWorkbook(t, []any{"031", "0000000031", "Uji Bersamaan", "", "8"})
	results := make([]*httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() { results[i] = call("operator", "commit", "", concurrent) })
	}
	wg.Wait()
	first, second := check(results[0], 200), check(results[1], 200)
	if first.Added+second.Added != 1 || len(first.Skipped)+len(second.Skipped) != 1 {
		t.Fatalf("unexpected concurrent results: %+v, %+v", first, second)
	}
	count(5)
}
