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
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPasswordHash(t *testing.T) {
	hash, err := passwordHash("a strong test password")
	if err != nil || !verifyPassword(hash, "a strong test password") || verifyPassword(hash, "wrong") {
		t.Fatal("password hashing failed", err)
	}
	if verifyPassword("invalid", "anything") {
		t.Fatal("invalid hash accepted")
	}
}

func TestSchoolDateAndCutoff(t *testing.T) {
	if !validClock("07:00") || validClock("24:00") || validClock("07:60") || validClock("7:00") {
		t.Fatal("clock validator mismatch")
	}
	now := time.Date(2026, 11, 16, 22, 1, 0, 0, time.UTC)
	day, minutes, err := dayAndMinutes(now, "Asia/Jayapura")
	if err != nil || day != "2026-11-17" || minutes-clockMinutes("07:00") != 1 {
		t.Fatalf("WIT boundary: %s %d %v", day, minutes, err)
	}
}

func TestPostgresAPI(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable database ending in hadir_stage1_test")
	}
	parsed, parseErr := url.Parse(dsn)
	if parseErr != nil || parsed.Path != "/hadir_stage1_test" {
		t.Fatal("refusing to run against a non-test database")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal("migrations must be idempotent:", err)
	}
	if _, err := db.Exec(`TRUNCATE attendance, sessions, users, people, settings RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if err := CreateAdmin(context.Background(), db, "admin@example.test", "a strong test password"); err != nil {
		t.Fatal(err)
	}
	s := NewServer(db, false)
	s.now = func() time.Time { return time.Date(2026, 11, 15, 0, 1, 0, 0, time.UTC) }
	call := func(method, path string, body any, cookie *http.Cookie) (int, map[string]any, *http.Cookie) {
		t.Helper()
		var reader *bytes.Reader
		if body == nil {
			reader = bytes.NewReader(nil)
		} else {
			raw, _ := json.Marshal(body)
			reader = bytes.NewReader(raw)
		}
		r := httptest.NewRequest(method, path, reader)
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
			t.Fatalf("%s %s: %v: %s", method, path, err, w.Body.String())
		}
		var set *http.Cookie
		for _, c := range w.Result().Cookies() {
			if c.Name == cookieName {
				set = c
				break
			}
		}
		return w.Code, out, set
	}
	if code, _, _ := call("GET", "/api/hadir", nil, nil); code != 401 {
		t.Fatalf("anonymous GET: %d", code)
	}
	code, login, adminCookie := call("POST", "/api/auth/login", map[string]any{"email": "admin@example.test", "password": "a strong test password"}, nil)
	if code != 200 || adminCookie == nil || adminCookie.Value == "" || login["user"] == nil {
		t.Fatalf("login: %d %#v", code, login)
	}
	if code, _, _ := call("POST", "/api/auth/login", map[string]any{"email": "admin@example.test", "password": "wrong"}, nil); code != 401 {
		t.Fatalf("bad login: %d", code)
	}
	code, _, _ = call("POST", "/api/users", map[string]any{"action": "create", "email": "gate@example.test", "name": "Petugas", "role": "petugas", "password": "a gate password 123"}, adminCookie)
	if code != 201 {
		t.Fatalf("create gate: %d", code)
	}
	code, _, _ = call("POST", "/api/users", map[string]any{"action": "create", "email": "operator@example.test", "name": "Operator", "role": "operator", "password": "an operator password 123"}, adminCookie)
	if code != 201 {
		t.Fatalf("create operator: %d", code)
	}
	code, _, operatorCookie := call("POST", "/api/auth/login", map[string]any{"email": "operator@example.test", "password": "an operator password 123"}, nil)
	if code != 200 || operatorCookie == nil {
		t.Fatalf("operator login: %d", code)
	}
	code, _, gateCookie := call("POST", "/api/auth/login", map[string]any{"email": "gate@example.test", "password": "a gate password 123"}, nil)
	if code != 200 || gateCookie == nil {
		t.Fatalf("gate login: %d", code)
	}
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "person", "name": "No", "number": "1", "role": "murid", "group": "VII A"}, gateCookie); code != 403 {
		t.Fatalf("gate could write people: %d", code)
	}
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "settings", "school": "Audit", "student_time": "07:00", "teacher_time": "06:45", "timezone": "Asia/Jakarta"}, operatorCookie); code != 403 {
		t.Fatalf("operator could change settings: %d", code)
	}
	if code, _, _ := call("GET", "/api/users", nil, operatorCookie); code != 403 {
		t.Fatalf("operator could list accounts: %d", code)
	}
	code, _, _ = call("POST", "/api/hadir", map[string]any{"action": "person", "name": "Alya", "number": "10234", "role": "murid", "group": "VII A"}, adminCookie)
	if code != 200 {
		t.Fatalf("create person: %d", code)
	}
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "person", "name": "Bima", "number": "10234", "role": "murid", "group": "VII A"}, operatorCookie); code != 409 {
		t.Fatalf("duplicate number: %d", code)
	}
	code, got, _ := call("GET", "/api/hadir", nil, adminCookie)
	people := got["people"].([]any)
	if code != 200 || len(people) != 1 || got["today"] != "2026-11-15" {
		t.Fatalf("GET: %d %#v", code, got)
	}
	p := people[0].(map[string]any)
	token := p["token"].(string)
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "person", "id": p["id"], "name": "Alya", "number": "10234", "role": "murid", "group": "VIII A"}, operatorCookie); code != 200 {
		t.Fatalf("operator edit: %d", code)
	}
	code, got, _ = call("GET", "/api/hadir", nil, adminCookie)
	updated := got["people"].([]any)[0].(map[string]any)
	if code != 200 || updated["token"] != token || updated["group_name"] != "VIII A" {
		t.Fatalf("edit changed token or group: %d %#v", code, updated)
	}
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "scan", "token": "HADIR:" + token, "mode": "pulang"}, gateCookie); code != 400 {
		t.Fatalf("early checkout: %d", code)
	}
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "scan", "token": token, "mode": "masuk"}, gateCookie); code != 200 {
		t.Fatalf("checkin: %d", code)
	}
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "scan", "token": token, "mode": "masuk"}, gateCookie); code != 409 {
		t.Fatalf("duplicate checkin: %d", code)
	}
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "lookup", "token": token}, gateCookie); code != 200 {
		t.Fatalf("lookup: %d", code)
	}
	var wg sync.WaitGroup
	checkout := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, _, _ := call("POST", "/api/hadir", map[string]any{"action": "scan", "token": token, "mode": "pulang"}, gateCookie)
			checkout <- c
		}()
	}
	wg.Wait()
	close(checkout)
	seen := map[int]int{}
	for c := range checkout {
		seen[c]++
	}
	if seen[200] != 1 || seen[409] != 1 {
		t.Fatalf("parallel checkout statuses: %#v", seen)
	}
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "active", "id": p["id"], "active": "false"}, adminCookie); code != 400 {
		t.Fatalf("string false accepted: %d", code)
	}
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "active", "id": "missing", "active": false}, adminCookie); code != 404 {
		t.Fatalf("unknown id: %d", code)
	}
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "active", "id": p["id"], "active": false}, adminCookie); code != 200 {
		t.Fatalf("deactivate: %d", code)
	}
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "lookup", "token": token}, gateCookie); code != 404 {
		t.Fatalf("disabled lookup: %d", code)
	}
	code, got, _ = call("GET", "/api/hadir", nil, adminCookie)
	if code != 200 || len(got["attendance"].([]any)) != 1 {
		t.Fatalf("history erased: %d %#v", code, got)
	}
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "demo"}, adminCookie); code != 200 {
		t.Fatalf("demo: %d", code)
	}
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "demo"}, adminCookie); code != 200 {
		t.Fatalf("demo repeat: %d", code)
	}
	code, got, _ = call("GET", "/api/hadir", nil, adminCookie)
	if code != 200 || len(got["people"].([]any)) != 16 {
		t.Fatalf("demo not idempotent: %d %#v", code, got)
	}
	if code, _, _ := call("POST", "/api/hadir", map[string]any{"action": "clearDemo"}, adminCookie); code != 200 {
		t.Fatalf("clearDemo: %d", code)
	}
	code, got, _ = call("GET", "/api/hadir", nil, gateCookie)
	if code != 200 || len(got["people"].([]any)) != 0 || len(got["attendance"].([]any)) != 0 {
		t.Fatalf("gate directory leak: %d %#v", code, got)
	}
	if code, _, _ := call("POST", "/api/auth/logout", map[string]any{}, adminCookie); code != 200 {
		t.Fatalf("logout: %d", code)
	}
	if code, _, _ := call("GET", "/api/auth/me", nil, adminCookie); code != 401 {
		t.Fatalf("old session active: %d", code)
	}
	code, users, _ := call("GET", "/api/users", nil, operatorCookie)
	if code != 403 || users["error"] == nil {
		t.Fatalf("operator access after admin logout: %d %#v", code, users)
	}
}
