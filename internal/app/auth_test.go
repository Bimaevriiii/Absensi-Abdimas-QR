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
	"strings"
	"testing"
)

func TestLoginLimiterAndUserGuards(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL")
	}
	if u, err := url.Parse(dsn); err != nil || u.Path != "/hadir_stage1_test" {
		t.Fatal("refusing to run against a non-test database")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`TRUNCATE attendance, sessions, users, people, settings RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	for _, e := range []string{"a@example.test", "b@example.test"} {
		if err := CreateAdmin(ctx, db, e, "a strong test password"); err != nil {
			t.Fatal(err)
		}
	}
	s := NewServer(db, false)
	call := func(path string, body map[string]any, cookie *http.Cookie) (int, *http.Cookie) {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		for _, c := range w.Result().Cookies() {
			if c.Name == cookieName {
				return w.Code, c
			}
		}
		return w.Code, nil
	}
	login := func(email, pw string) (int, *http.Cookie) {
		return call("/api/auth/login", map[string]any{"email": email, "password": pw}, nil)
	}

	// Limiter: 5 failures (even for an unknown email), then 429, but not for another email from the same IP.
	for i := 0; i < maxFails; i++ {
		if code, _ := login("nobody@example.test", "wrong"); code != 401 {
			t.Fatalf("fail %d: %d", i, code)
		}
	}
	if code, _ := login("nobody@example.test", "wrong"); code != 429 {
		t.Fatalf("expected 429, got %d", code)
	}
	if code, _ := login("a@example.test", "a strong test password"); code == 429 {
		t.Fatalf("one email's lockout must not block other users on the same IP")
	}
	s.fails = map[string]failure{}
	if code, _ := login("a@example.test", strings.Repeat("x", maxPassword+1)); code != 400 {
		t.Fatalf("overlong password: %d", code)
	}
	_, ca := login("a@example.test", "a strong test password")
	_, cb := login("b@example.test", "a strong test password")
	if ca == nil || cb == nil {
		t.Fatal("login failed")
	}

	var idA, idB string
	db.QueryRow(`SELECT id FROM users WHERE email='a@example.test'`).Scan(&idA)
	db.QueryRow(`SELECT id FROM users WHERE email='b@example.test'`).Scan(&idB)
	act := func(c *http.Cookie, action, id, role string) int {
		code, _ := call("/api/users", map[string]any{"action": action, "id": id, "role": role}, c)
		return code
	}
	if code := act(ca, "setRole", idB, "operator"); code != 200 {
		t.Fatalf("demote b: %d", code)
	}
	if code := act(ca, "setRole", idA, "operator"); code != 409 {
		t.Fatalf("demote last pengelola: %d", code)
	}
	if code := act(ca, "setRole", idB, "pengelola"); code != 200 {
		t.Fatalf("promote b: %d", code)
	}
	if code := act(cb, "deactivate", idA, ""); code != 200 {
		t.Fatalf("deactivate a: %d", code)
	}
	if code := act(cb, "setRole", idB, "petugas"); code != 409 {
		t.Fatalf("demote last active: %d", code)
	}
	if code := act(cb, "activate", idA, ""); code != 200 {
		t.Fatalf("activate a: %d", code)
	}
	if code := act(cb, "activate", "missing", ""); code != 404 {
		t.Fatalf("activate unknown: %d", code)
	}
}
