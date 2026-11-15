package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/Bimaevriiii/Absensi-Abdimas-QR-Go-PWA/internal/app"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: hadir serve|migrate|create-admin")
	}
	dsn, err := databaseDSN()
	if err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatal(err)
	}
	switch os.Args[1] {
	case "migrate":
		err = app.Migrate(context.Background(), db)
	case "create-admin":
		err = app.CreateAdmin(context.Background(), db, os.Getenv("ADMIN_EMAIL"), os.Getenv("ADMIN_PASSWORD"))
	case "serve":
		addr := os.Getenv("HTTP_ADDR")
		if addr == "" {
			addr = "127.0.0.1:8080"
		}
		handler := app.NewServer(db, os.Getenv("COOKIE_SECURE") != "false")
		server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 25 * time.Second, IdleTimeout: 60 * time.Second}
		log.Printf("Hadir API listening on %s", addr)
		err = server.ListenAndServe()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func databaseDSN() (string, error) {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn, nil
	}
	host := os.Getenv("DB_HOST")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	name := os.Getenv("DB_NAME")
	if host == "" || user == "" || password == "" || name == "" {
		return "", fmt.Errorf("set DATABASE_URL or DB_HOST, DB_USER, DB_PASSWORD, and DB_NAME")
	}
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5432"
	}
	sslmode := os.Getenv("DB_SSLMODE")
	if sslmode == "" {
		sslmode = "disable"
	}
	query := url.Values{"sslmode": {sslmode}}
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, password),
		Host:     net.JoinHostPort(host, port),
		Path:     "/" + name,
		RawQuery: query.Encode(),
	}
	return u.String(), nil
}
