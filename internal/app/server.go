package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type Server struct {
	db           *sql.DB
	cookieSecure bool
	now          func() time.Time
	mu           sync.Mutex
	fails        map[string]failure // login failures keyed by "ip:..." and "email:..."
	hashSem      chan struct{}      // bounds concurrent 64MB argon2 runs
}

func NewServer(db *sql.DB, cookieSecure bool) *Server {
	return &Server{db: db, cookieSecure: cookieSecure, now: time.Now, fails: map[string]failure{}, hashSem: make(chan struct{}, 4)}
}

func (s *Server) Handler() http.Handler { return s }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method == http.MethodPost {
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "Asal permintaan tidak sesuai."})
				return
			}
		}
		if r.URL.Path != "/api/students/import" && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Kirim data JSON."})
			return
		}
	}
	switch r.URL.Path {
	case "/healthz":
		if r.Method == http.MethodGet {
			writeJSON(w, 200, map[string]string{"status": "ok"})
			return
		}
	case "/readyz":
		if r.Method == http.MethodGet {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := s.db.PingContext(ctx); err != nil {
				writeJSON(w, 503, map[string]string{"error": "Database belum siap."})
				return
			}
			writeJSON(w, 200, map[string]string{"status": "ok"})
			return
		}
	case "/api/auth/login":
		if r.Method == http.MethodPost {
			s.login(w, r)
			return
		}
	case "/api/auth/logout":
		if r.Method == http.MethodPost {
			s.logout(w, r)
			return
		}
	case "/api/auth/me":
		if r.Method == http.MethodGet {
			s.me(w, r)
			return
		}
	case "/api/users":
		if r.Method == http.MethodGet || r.Method == http.MethodPost {
			s.users(w, r)
			return
		}
	case "/api/hadir":
		if r.Method == http.MethodGet || r.Method == http.MethodPost {
			s.hadir(w, r)
			return
		}
	case "/api/students":
		if r.Method == http.MethodGet {
			s.studentsGet(w, r)
			return
		}
	case "/api/students/import":
		if r.Method == http.MethodPost {
			s.importStudents(w, r)
			return
		}
	case "/api/students/import/template":
		if r.Method == http.MethodGet {
			s.downloadStudentTemplate(w, r)
			return
		}
	case "/api/audit":
		if r.Method == http.MethodGet {
			s.auditGet(w, r)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "Halaman tidak ditemukan."})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func decodeJSON(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("JSON tambahan tidak diperbolehkan")
	}
	return nil
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	log.Printf("backend error: %v", err)
	writeJSON(w, 500, map[string]string{"error": "Terjadi kesalahan server. Coba lagi."})
}

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request, roles ...string) (User, bool) {
	u, err := s.sessionUser(r.Context(), r)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, 401, map[string]string{"error": "Silakan masuk untuk mengakses absensi."})
		return User{}, false
	}
	if err != nil {
		s.fail(w, err)
		return User{}, false
	}
	for _, role := range roles {
		if role == u.Role {
			return u, true
		}
	}
	writeJSON(w, 403, map[string]string{"error": "Akun Anda tidak memiliki izin untuk tindakan ini."})
	return User{}, false
}

type loginBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var b loginBody
	if decodeJSON(r, &b) != nil {
		writeJSON(w, 400, map[string]string{"error": "Periksa email dan kata sandi."})
		return
	}
	email := strings.ToLower(strings.TrimSpace(b.Email))
	if len(b.Password) > maxPassword || len(email) > 254 {
		writeJSON(w, 400, map[string]string{"error": "Periksa email dan kata sandi."})
		return
	}
	ipKey, emailKey := "ip:"+s.clientIP(r), "email:"+email
	if s.blocked(ipKey, maxIPFails) || s.blocked(emailKey, maxFails) {
		writeJSON(w, 429, map[string]string{"error": "Terlalu banyak percobaan. Coba lagi nanti."})
		return
	}
	var u User
	var hash string
	err := s.db.QueryRowContext(r.Context(), `SELECT id,email,name,role,password_hash FROM users WHERE email=$1 AND active=true`, email).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		hash, err = dummyHash(), nil // equalize timing with the known-user path
		u = User{}
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	if !s.verify(hash, b.Password) || u.ID == "" {
		s.addFails(ipKey, emailKey)
		writeJSON(w, 401, map[string]string{"error": "Email atau kata sandi tidak sesuai."})
		return
	}
	s.clearFails(emailKey)
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE expires_at<$1`, s.now()); err != nil {
		log.Printf("session purge: %v", err)
	}
	token, err := newSecret()
	if err != nil {
		s.fail(w, err)
		return
	}
	expires := s.now().Add(sessionTTL)
	if _, err := s.db.ExecContext(r.Context(), `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)`, tokenHash(token), u.ID, expires); err != nil {
		s.fail(w, err)
		return
	}
	s.setSessionCookie(w, token, expires)
	writeJSON(w, 200, map[string]any{"user": u})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil && len(c.Value) == 64 {
		if _, err := s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE token_hash=$1`, tokenHash(c.Value)); err != nil {
			s.fail(w, err)
			return
		}
	}
	s.clearSessionCookie(w)
	writeJSON(w, 200, map[string]string{"message": "Anda telah keluar."})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r, "pengelola", "operator", "petugas")
	if ok {
		writeJSON(w, 200, map[string]any{"user": u})
	}
}

type userBody struct {
	Action   string `json:"action"`
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireUser(w, r, "pengelola")
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		rows, err := s.db.QueryContext(r.Context(), `SELECT id,email,name,role,active FROM users ORDER BY email`)
		if err != nil {
			s.fail(w, err)
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id, email, name, role string
			var active bool
			if err := rows.Scan(&id, &email, &name, &role, &active); err != nil {
				s.fail(w, err)
				return
			}
			out = append(out, map[string]any{"id": id, "email": email, "name": name, "role": role, "active": active})
		}
		if err := rows.Err(); err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"users": out})
		return
	}
	var b userBody
	if decodeJSON(r, &b) != nil {
		writeJSON(w, 400, map[string]string{"error": "Periksa data akun."})
		return
	}
	switch b.Action {
	case "create":
		email := strings.ToLower(strings.TrimSpace(b.Email))
		name := strings.TrimSpace(b.Name)
		address, parseErr := mail.ParseAddress(email)
		if parseErr != nil || address.Address != email || len(email) > 254 || name == "" || len(name) > 100 || len(b.Password) < 12 || len(b.Password) > maxPassword || (b.Role != "pengelola" && b.Role != "operator" && b.Role != "petugas") {
			writeJSON(w, 400, map[string]string{"error": "Periksa email, nama, peran, dan kata sandi minimal 12 dan maksimal 128 karakter."})
			return
		}
		id, err := newID()
		if err != nil {
			s.fail(w, err)
			return
		}
		hash, err := passwordHash(b.Password)
		if err != nil {
			s.fail(w, err)
			return
		}
		_, err = s.db.ExecContext(r.Context(), `INSERT INTO users(id,email,name,role,password_hash) VALUES($1,$2,$3,$4,$5)`, id, email, name, b.Role, hash)
		if uniqueViolation(err) {
			writeJSON(w, 409, map[string]string{"error": "Email sudah digunakan."})
			return
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		log.Printf("audit: %s created user %s role=%s", actor.Email, email, b.Role)
		writeJSON(w, 201, map[string]any{"user": User{ID: id, Email: email, Name: name, Role: b.Role}})
	case "deactivate":
		if b.ID == "" || b.ID == actor.ID {
			writeJSON(w, 400, map[string]string{"error": "Pilih akun lain yang akan dinonaktifkan."})
			return
		}
		if !s.setUser(w, r, b.ID, true, `UPDATE users SET active=false WHERE id=$1`) {
			return
		}
		if _, err := s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, b.ID); err != nil {
			s.fail(w, err)
			return
		}
		log.Printf("audit: %s deactivated user %s", actor.Email, b.ID)
		writeJSON(w, 200, map[string]string{"message": "Akun dinonaktifkan."})
	case "activate":
		if b.ID != "" && s.setUser(w, r, b.ID, false, `UPDATE users SET active=true WHERE id=$1`) {
			log.Printf("audit: %s activated user %s", actor.Email, b.ID)
			writeJSON(w, 200, map[string]string{"message": "Akun diaktifkan."})
		} else if b.ID == "" {
			writeJSON(w, 400, map[string]string{"error": "ID akun diperlukan."})
		}
	case "setRole":
		if b.Role != "pengelola" && b.Role != "operator" && b.Role != "petugas" {
			writeJSON(w, 400, map[string]string{"error": "Peran tidak valid."})
			return
		}
		if b.ID != "" && s.setUser(w, r, b.ID, b.Role != "pengelola", `UPDATE users SET role=$2 WHERE id=$1`, b.Role) {
			log.Printf("audit: %s set role of user %s to %s", actor.Email, b.ID, b.Role)
			writeJSON(w, 200, map[string]string{"message": "Peran diperbarui."})
		} else if b.ID == "" {
			writeJSON(w, 400, map[string]string{"error": "ID akun diperlukan."})
		}
	case "resetPassword":
		if b.ID == "" || len(b.Password) < 12 || len(b.Password) > maxPassword {
			writeJSON(w, 400, map[string]string{"error": "ID dan kata sandi 12-128 karakter diperlukan."})
			return
		}
		hash, err := passwordHash(b.Password)
		if err != nil {
			s.fail(w, err)
			return
		}
		res, err := s.db.ExecContext(r.Context(), `UPDATE users SET password_hash=$1 WHERE id=$2`, hash, b.ID)
		if err != nil {
			s.fail(w, err)
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			writeJSON(w, 404, map[string]string{"error": "Akun tidak ditemukan."})
			return
		}
		_, err = s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, b.ID)
		if err != nil {
			s.fail(w, err)
			return
		}
		log.Printf("audit: %s reset password of user %s", actor.Email, b.ID)
		writeJSON(w, 200, map[string]string{"message": "Kata sandi diperbarui; sesi lama berakhir."})
	default:
		writeJSON(w, 400, map[string]string{"error": "Tindakan tidak dikenal."})
	}
}

func uniqueViolation(err error) bool {
	var p *pgconn.PgError
	return errors.As(err, &p) && p.Code == "23505"
}

// setUser runs update (args: id, extra...) in a tx serialized by an advisory lock.
// If demote, it refuses (409) to remove the last active pengelola. Writes the error response and returns false on failure.
func (s *Server) setUser(w http.ResponseWriter, r *http.Request, id string, demote bool, update string, extra ...any) bool {
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.fail(w, err)
		return false
	}
	defer tx.Rollback()
	var role string
	var active bool
	var n int
	if _, err = tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(7001)`); err == nil {
		err = tx.QueryRowContext(r.Context(), `SELECT role,active FROM users WHERE id=$1`, id).Scan(&role, &active)
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, 404, map[string]string{"error": "Akun tidak ditemukan."})
		return false
	}
	if err == nil && demote && role == "pengelola" && active {
		err = tx.QueryRowContext(r.Context(), `SELECT count(*) FROM users WHERE role='pengelola' AND active=true`).Scan(&n)
		if err == nil && n <= 1 {
			writeJSON(w, 409, map[string]string{"error": "Minimal harus ada satu pengelola aktif."})
			return false
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), update, append([]any{id}, extra...)...)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		s.fail(w, err)
		return false
	}
	return true
}
