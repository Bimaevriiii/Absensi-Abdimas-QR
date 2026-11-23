package app

import (
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// studentProfile is returned only by the directory endpoint, never by attendance lookups.
type studentProfile struct {
	NISN          string `json:"nisn"`
	Gender        string `json:"gender"`
	ClassParallel string `json:"class_parallel"`
	BirthPlace    string `json:"birth_place"`
	BirthDate     string `json:"birth_date"`
	NIK           string `json:"nik"`
	HomeAddress   string `json:"home_address"`
}

var decimalDigits = regexp.MustCompile(`^[0-9]+$`)

func (p *studentProfile) normalize() (column, message string) {
	for _, field := range []struct {
		column string
		value  *string
	}{
		{"NISN", &p.NISN}, {"L/P", &p.Gender}, {"KELAS PARAREL", &p.ClassParallel},
		{"TEMPAT/TANGGAL LAHIR", &p.BirthPlace}, {"TEMPAT/TANGGAL LAHIR", &p.BirthDate},
		{"NIK SISWA", &p.NIK}, {"ALAMAT RUMAH", &p.HomeAddress},
	} {
		*field.value = strings.TrimSpace(*field.value)
		if strings.ContainsRune(*field.value, 0) {
			return field.column, "Biodata murid memuat karakter tidak valid."
		}
	}
	if p.NISN != "" && (len(p.NISN) != 10 || !decimalDigits.MatchString(p.NISN)) {
		return "NISN", "NISN harus berisi tepat 10 digit."
	}
	if p.NIK != "" && (len(p.NIK) != 16 || !decimalDigits.MatchString(p.NIK)) {
		return "NIK SISWA", "NIK siswa harus berisi tepat 16 digit."
	}
	if p.Gender != "" && p.Gender != "L" && p.Gender != "P" {
		return "L/P", "Jenis kelamin harus L atau P."
	}
	if p.BirthDate != "" {
		day, err := time.Parse(time.DateOnly, p.BirthDate)
		if err != nil || day.Year() < 1 || day.Format(time.DateOnly) != p.BirthDate {
			return "TEMPAT/TANGGAL LAHIR", "Tanggal lahir harus berupa tanggal yang valid."
		}
	}
	if utf8.RuneCountInString(p.ClassParallel) > 60 {
		return "KELAS PARAREL", "Kelas paralel maksimal 60 karakter."
	}
	if utf8.RuneCountInString(p.BirthPlace) > 100 {
		return "TEMPAT/TANGGAL LAHIR", "Tempat lahir maksimal 100 karakter."
	}
	if utf8.RuneCountInString(p.HomeAddress) > 500 {
		return "ALAMAT RUMAH", "Alamat rumah maksimal 500 karakter."
	}
	return "", ""
}

func (s *Server) studentsGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireUser(w, r, "pengelola", "operator"); !ok {
		return
	}
	type directoryProfile struct {
		ID string `json:"id"`
		studentProfile
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, nisn, gender, class_parallel, birth_place,
		COALESCE(to_char(birth_date, 'YYYY-MM-DD'), ''), nik, home_address FROM people WHERE role='murid'`)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer rows.Close()
	students := []directoryProfile{}
	for rows.Next() {
		var p directoryProfile
		if err := rows.Scan(&p.ID, &p.NISN, &p.Gender, &p.ClassParallel, &p.BirthPlace, &p.BirthDate, &p.NIK, &p.HomeAddress); err != nil {
			s.fail(w, err)
			return
		}
		students = append(students, p)
	}
	if err := rows.Err(); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"students": students})
}
