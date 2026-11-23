package app

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"mime"
	"net/http"
	"sort"
	"time"
)

const maxStudentUpload = 1 << 20

type studentImportResult struct {
	New     int                  `json:"new"`
	Added   int                  `json:"added"`
	Skipped []studentImportIssue `json:"skipped"`
	Errors  []studentImportIssue `json:"errors"`
}

func (s *Server) downloadStudentTemplate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireUser(w, r, "pengelola", "operator"); !ok {
		return
	}
	book, err := studentTemplate()
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", xlsxMediaType)
	w.Header().Set("Content-Disposition", `attachment; filename="contoh-impor-murid.xlsx"`)
	_, _ = book.WriteTo(w)
}

func (s *Server) importStudents(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireUser(w, r, "pengelola", "operator"); !ok {
		return
	}
	mode := r.URL.Query().Get("mode")
	if mode != "preview" && mode != "commit" {
		writeJSON(w, 400, map[string]string{"error": "Pilih mode preview atau commit."})
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != xlsxMediaType {
		writeJSON(w, 415, map[string]string{"error": "Pilih file Excel dengan ekstensi .xlsx."})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxStudentUpload))
	if err != nil {
		var sizeError *http.MaxBytesError
		if errors.As(err, &sizeError) {
			writeJSON(w, 413, map[string]string{"error": "Ukuran file maksimal 1 MiB."})
		} else {
			writeJSON(w, 400, map[string]string{"error": "Unggahan gagal dibaca."})
		}
		return
	}
	students, issues, err := readStudentWorkbook(bytes.NewReader(body))
	if err != nil {
		writeJSON(w, 422, map[string]string{"error": err.Error()})
		return
	}
	result := studentImportResult{Skipped: []studentImportIssue{}, Errors: append([]studentImportIssue{}, issues...)}
	ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
	defer cancel()

	var query studentImportQuerier = s.db
	var tx *sql.Tx
	if mode == "commit" && len(result.Errors) == 0 {
		tx, err = s.db.BeginTx(ctx, nil)
		if err != nil {
			s.fail(w, err)
			return
		}
		defer tx.Rollback()
		// Serialize identity changes through revalidation and insertion while
		// allowing ordinary directory reads to continue.
		if _, err := tx.ExecContext(ctx, "LOCK TABLE people IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			s.fail(w, err)
			return
		}
		query = tx
	}

	pending, err := checkStudentImports(ctx, query, students, &result)
	if err != nil {
		s.fail(w, err)
		return
	}
	result.New = len(pending)
	sort.SliceStable(result.Errors, func(i, j int) bool { return result.Errors[i].Row < result.Errors[j].Row })
	if mode == "preview" {
		writeJSON(w, 200, result)
		return
	}
	if len(result.Errors) > 0 {
		writeJSON(w, 422, struct {
			studentImportResult
			Error string `json:"error"`
		}{result, "Perbaiki baris yang bermasalah sebelum menyimpan."})
		return
	}
	for _, student := range pending {
		if err := insertPerson(ctx, tx, student.name, student.number, "murid", student.group, student.profile); err != nil {
			s.fail(w, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.fail(w, err)
		return
	}
	result.Added = len(pending)
	writeJSON(w, 200, result)
}

type studentImportQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func checkStudentImports(ctx context.Context, db studentImportQuerier, students []studentImportRow, result *studentImportResult) ([]studentImportRow, error) {
	if len(students) == 0 {
		return nil, nil
	}
	numbers := make([]string, 0, len(students))
	nisns := make([]string, 0, len(students))
	for _, student := range students {
		numbers = append(numbers, student.number)
		if student.profile.NISN != "" {
			nisns = append(nisns, student.profile.NISN)
		}
	}

	rows, err := db.QueryContext(ctx, `SELECT id, role, number, nisn FROM people
		WHERE number=ANY($1::text[]) OR (role='murid' AND nisn<>'' AND nisn=ANY($2::text[]))`, numbers, nisns)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type identity struct{ id, role string }
	byNumber := map[string]identity{}
	byNISN := map[string]string{}
	for rows.Next() {
		var matched identity
		var number, nisn string
		if err := rows.Scan(&matched.id, &matched.role, &number, &nisn); err != nil {
			return nil, err
		}
		byNumber[number] = matched
		if matched.role == "murid" && nisn != "" {
			byNISN[nisn] = matched.id
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var pending []studentImportRow
	for _, student := range students {
		local, nisn := byNumber[student.number], byNISN[student.profile.NISN]
		switch {
		case local.role == "guru":
			result.Errors = append(result.Errors, studentImportIssue{student.line, "NIS LOKAL", "Nomor ini sudah digunakan sebagai NIP guru."})
		case local.id != "" && nisn != "" && local.id != nisn:
			result.Errors = append(result.Errors, studentImportIssue{student.line, "NIS LOKAL / NISN", "Kedua nomor menunjuk murid yang berbeda."})
		case local.id != "":
			result.Skipped = append(result.Skipped, studentImportIssue{student.line, "NIS LOKAL", "Murid dengan NIS Lokal ini sudah terdaftar."})
		case nisn != "":
			result.Skipped = append(result.Skipped, studentImportIssue{student.line, "NISN", "Murid dengan NISN ini sudah terdaftar."})
		default:
			pending = append(pending, student)
		}
	}
	return pending, nil
}
