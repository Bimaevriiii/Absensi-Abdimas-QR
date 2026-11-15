package main

import (
	"net/url"
	"strings"
	"testing"
)

func TestDatabaseDSNFromPartsEscapesCredentials(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_HOST", "db")
	t.Setenv("DB_USER", "hadir")
	t.Setenv("DB_PASSWORD", "p@ss:/?#word")
	t.Setenv("DB_NAME", "hadir")
	t.Setenv("DB_PORT", "")
	t.Setenv("DB_SSLMODE", "")

	dsn, err := databaseDSN()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := u.User.Password()
	if u.Host != "db:5432" || u.User.Username() != "hadir" || password != "p@ss:/?#word" || u.Path != "/hadir" || u.Query().Get("sslmode") != "disable" {
		t.Fatalf("unexpected database URL components: host=%q user=%q password=%q path=%q query=%q", u.Host, u.User.Username(), password, u.Path, u.RawQuery)
	}
}

func TestDatabaseDSNPrefersExplicitURL(t *testing.T) {
	want := "postgres://custom@localhost/mydb?sslmode=require"
	t.Setenv("DATABASE_URL", want)
	dsn, err := databaseDSN()
	if err != nil || dsn != want {
		t.Fatalf("databaseDSN() = %q, %v; want %q", dsn, err, want)
	}
}

func TestDatabaseDSNRequiresCredentials(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_HOST", "db")
	t.Setenv("DB_USER", "hadir")
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_NAME", "hadir")
	_, err := databaseDSN()
	if err == nil || !strings.Contains(err.Error(), "DB_PASSWORD") {
		t.Fatalf("expected missing database credentials error, got %v", err)
	}
}
