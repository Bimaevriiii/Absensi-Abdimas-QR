package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Person struct {
	ID        string `json:"id"`
	Token     string `json:"token"`
	Name      string `json:"name"`
	Number    string `json:"number"`
	Role      string `json:"role"`
	GroupName string `json:"group_name"`
	Active    int    `json:"active"`
	Demo      int    `json:"demo"`
}

type Attendance struct {
	ID          string     `json:"id"`
	PersonID    string     `json:"person_id"`
	Day         string     `json:"day"`
	Arrived     *time.Time `json:"arrived"`
	Departed    *time.Time `json:"departed"`
	Status      string     `json:"status"`
	LateMinutes int        `json:"late_minutes"`
	Note        string     `json:"note"`
	Source      string     `json:"source"`
}

type Settings struct {
	School               string `json:"school"`
	StudentTime          string `json:"student_time"`
	TeacherTime          string `json:"teacher_time"`
	StudentDepartureTime string `json:"student_departure_time"`
	TeacherDepartureTime string `json:"teacher_departure_time"`
	Timezone             string `json:"timezone"`
}

var defaults = Settings{School: "Sekolah Kita", StudentTime: "07:00", TeacherTime: "06:45", StudentDepartureTime: "13:00", TeacherDepartureTime: "14:00", Timezone: "Asia/Jakarta"}

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func newSecret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func Migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(84260015)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		var applied bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name=$1)`, entry.Name()).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		body, err := migrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("migration %s: %w", entry.Name(), err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(name) VALUES($1)`, entry.Name()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func readSettings(ctx context.Context, db *sql.DB) (Settings, error) {
	var cfg Settings
	err := db.QueryRowContext(ctx, `SELECT school,student_time,teacher_time,student_departure_time,teacher_departure_time,timezone FROM settings WHERE id='main'`).Scan(&cfg.School, &cfg.StudentTime, &cfg.TeacherTime, &cfg.StudentDepartureTime, &cfg.TeacherDepartureTime, &cfg.Timezone)
	if errors.Is(err, sql.ErrNoRows) {
		return defaults, nil
	}
	return cfg, err
}

func dayAndMinutes(now time.Time, timezone string) (string, int, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return "", 0, err
	}
	local := now.In(loc)
	return local.Format("2006-01-02"), local.Hour()*60 + local.Minute(), nil
}

func validClock(v string) bool {
	if len(v) != 5 || v[2] != ':' {
		return false
	}
	for _, i := range []int{0, 1, 3, 4} {
		if v[i] < '0' || v[i] > '9' {
			return false
		}
	}
	return (int(v[0]-'0')*10+int(v[1]-'0')) <= 23 && (int(v[3]-'0')*10+int(v[4]-'0')) <= 59
}

func clockMinutes(v string) int {
	return (int(v[0]-'0')*10+int(v[1]-'0'))*60 + int(v[3]-'0')*10 + int(v[4]-'0')
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
