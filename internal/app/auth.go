package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

const cookieName = "hadir_session"
const sessionTTL = 12 * time.Hour
const maxPassword = 128
const maxFails = 5    // per email
const maxIPFails = 30 // per IP; higher because a whole school can share one NAT address
const failWindow = 15 * time.Minute

// dummyHash is verified when the email is unknown so response time doesn't reveal accounts.
var dummyHash = sync.OnceValue(func() string { h, _ := passwordHash("dummy password for timing"); return h })

type failure struct {
	n     int
	until time.Time
}

// ponytail: in-memory per-process limiter, resets on restart and isn't shared across instances; move to Postgres/Redis if scaled out.
func (s *Server) blocked(key string, max int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.fails[key]
	return f.n >= max && s.now().Before(f.until)
}

func (s *Server) addFails(keys ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if len(s.fails) > 10000 {
		for k, f := range s.fails {
			if now.After(f.until) {
				delete(s.fails, k)
			}
		}
	}
	for _, k := range keys {
		f := s.fails[k]
		if now.After(f.until) {
			f.n = 0
		}
		s.fails[k] = failure{f.n + 1, now.Add(failWindow)}
	}
}

func (s *Server) clearFails(key string) {
	s.mu.Lock()
	delete(s.fails, key)
	s.mu.Unlock()
}

func (s *Server) verify(hash, password string) bool {
	s.hashSem <- struct{}{}
	defer func() { <-s.hashSem }()
	return verifyPassword(hash, password)
}

// clientIP trusts X-Forwarded-For only from a private/loopback peer, and takes the
// rightmost non-private entry (the first entry is client-controlled with nginx's $proxy_add_x_forwarded_for).
func (s *Server) clientIP(r *http.Request) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLoopback()) {
		parts := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
		for i := len(parts) - 1; i >= 0; i-- {
			if p := net.ParseIP(strings.TrimSpace(parts[i])); p != nil && !p.IsPrivate() && !p.IsLoopback() {
				return p.String()
			}
		}
	}
	return host
}

func passwordHash(password string) (string, error) {
	var salt [16]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt[:], 3, 64*1024, 2, 32)
	return fmt.Sprintf("argon2id$v=19$m=65536,t=3,p=2$%s$%s", base64.RawStdEncoding.EncodeToString(salt[:]), base64.RawStdEncoding.EncodeToString(key)), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" || parts[1] != "v=19" || parts[2] != "m=65536,t=3,p=2" {
		return false
	}
	salt, e1 := base64.RawStdEncoding.DecodeString(parts[3])
	key, e2 := base64.RawStdEncoding.DecodeString(parts[4])
	if e1 != nil || e2 != nil || len(salt) != 16 || len(key) != 32 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(actual, key) == 1
}

func CreateAdmin(ctx context.Context, db *sql.DB, email, password string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(password) < 12 {
		return errors.New("ADMIN_EMAIL must be a valid email and ADMIN_PASSWORD must contain at least 12 characters")
	}
	hash, err := passwordHash(password)
	if err != nil {
		return err
	}
	id, err := newID()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO users(id,email,name,role,password_hash) VALUES($1,$2,$3,'pengelola',$4)`, id, email, "Pengelola", hash)
	return err
}

func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (s *Server) sessionUser(ctx context.Context, req *http.Request) (User, error) {
	cookie, err := req.Cookie(cookieName)
	if err != nil || len(cookie.Value) != 64 {
		return User{}, sql.ErrNoRows
	}
	var u User
	err = s.db.QueryRowContext(ctx, `SELECT u.id,u.email,u.name,u.role FROM sessions x JOIN users u ON u.id=x.user_id WHERE x.token_hash=$1 AND x.expires_at>$2 AND u.active=true`, tokenHash(cookie.Value), s.now()).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
	return u, err
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", Expires: expires, HttpOnly: true, Secure: s.cookieSecure, SameSite: http.SameSiteStrictMode})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.cookieSecure, SameSite: http.SameSiteStrictMode})
}
