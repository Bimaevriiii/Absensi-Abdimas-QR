package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

const (
	studentSheet  = "Murid"
	xlsxMediaType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	maxImportRows = 5000
)

var studentColumns = [...]string{
	"NIS LOKAL", "NISN", "NAMA SISWA", "L/P", "KLS", "KELAS PARAREL", "TEMPAT/TANGGAL LAHIR", "NIK SISWA", "ALAMAT RUMAH",
}

type studentImportRow struct {
	line                int
	name, number, group string
	profile             studentProfile
}

type studentImportIssue struct {
	Row    int    `json:"row"`
	Column string `json:"column"`
	Reason string `json:"reason"`
}

func readStudentWorkbook(body io.Reader) ([]studentImportRow, []studentImportIssue, error) {
	book, err := excelize.OpenReader(body, excelize.Options{
		RawCellValue:      true,
		UnzipSizeLimit:    16 << 20,
		UnzipXMLSizeLimit: 16 << 20,
	})
	if err != nil {
		return nil, nil, errors.New("File Excel tidak dapat dibaca. Gunakan file .xlsx yang valid.")
	}
	defer book.Close()

	if !slices.Contains(book.GetSheetList(), studentSheet) {
		return nil, nil, errors.New("Sheet Murid tidak ditemukan. Gunakan contoh Excel dari aplikasi.")
	}
	rows, err := book.Rows(studentSheet)
	if err != nil {
		return nil, nil, errors.New("Sheet Murid tidak dapat dibaca.")
	}
	defer rows.Close()

	// Check the worksheet bounds before looking up individual cell types.
	var input []struct {
		line   int
		values [len(studentColumns)]string
	}
	var issues []studentImportIssue
	sheetReadError := errors.New("Isi sheet Murid tidak dapat dibaca.")
	for line := 1; rows.Next(); line++ {
		if line > maxImportRows+1 {
			return nil, nil, fmt.Errorf("Gunakan maksimal %d baris data per file.", maxImportRows)
		}
		columns, err := rows.Columns()
		if err != nil {
			return nil, nil, sheetReadError
		}
		for i, value := range columns {
			columns[i] = strings.TrimSpace(value)
		}
		for len(columns) > 0 && columns[len(columns)-1] == "" {
			columns = columns[:len(columns)-1]
		}
		if line == 1 {
			for i, value := range columns {
				columns[i] = strings.ToUpper(value)
			}
			if !slices.Equal(columns, studentColumns[:]) {
				return nil, nil, errors.New("Baris 1 harus berisi sembilan judul kolom sesuai contoh Excel.")
			}
			continue
		}
		if len(columns) == 0 {
			continue
		}
		if len(columns) > len(studentColumns) {
			issues = append(issues, studentImportIssue{line, "J dan seterusnya", "Gunakan sembilan kolom sesuai contoh Excel."})
			continue
		}
		var values [len(studentColumns)]string
		copy(values[:], columns)
		input = append(input, struct {
			line   int
			values [len(studentColumns)]string
		}{line, values})
	}
	if rows.Error() != nil {
		return nil, nil, sheetReadError
	}
	if len(input) == 0 && len(issues) == 0 {
		return nil, nil, errors.New("Sheet Murid belum berisi data.")
	}

	var students []studentImportRow
	numbers, nisns := map[string]int{}, map[string]int{}
	for _, row := range input {
		student, issue, err := readStudentRow(book, row.line, row.values)
		if err != nil {
			return nil, nil, sheetReadError
		}
		if issue.Reason != "" {
			issues = append(issues, issue)
			continue
		}

		issueCount := len(issues)
		if previous, exists := numbers[student.number]; exists {
			issues = append(issues, duplicateWorkbookIssue(row.line, "NIS LOKAL", previous))
		} else {
			numbers[student.number] = row.line
		}
		if student.profile.NISN != "" {
			if previous, exists := nisns[student.profile.NISN]; exists {
				issues = append(issues, duplicateWorkbookIssue(row.line, "NISN", previous))
			} else {
				nisns[student.profile.NISN] = row.line
			}
		}
		if len(issues) == issueCount {
			students = append(students, student)
		}
	}
	return students, issues, nil
}

func duplicateWorkbookIssue(line int, column string, previous int) studentImportIssue {
	return studentImportIssue{line, column, fmt.Sprintf("Nomor ini juga digunakan pada baris %d di file.", previous)}
}

func readStudentRow(book *excelize.File, line int, values [len(studentColumns)]string) (studentImportRow, studentImportIssue, error) {
	student := studentImportRow{line: line, name: values[2], number: values[0], group: values[4]}
	if column, message := validatePersonIdentity(student.name, student.number, student.group); message != "" {
		return student, studentImportIssue{line, column, message}, nil
	}
	for _, i := range []int{0, 1, 7} {
		if values[i] == "" {
			continue
		}
		cell, err := excelize.CoordinatesToCellName(i+1, line)
		if err != nil {
			return student, studentImportIssue{}, err
		}
		cellType, err := book.GetCellType(studentSheet, cell)
		if err != nil {
			return student, studentImportIssue{}, err
		}
		if cellType != excelize.CellTypeSharedString && cellType != excelize.CellTypeInlineString {
			message := "Atur sel sebagai Teks, lalu salin ulang nomor dari sumber aslinya."
			return student, studentImportIssue{line, studentColumns[i], message}, nil
		}
	}
	place, date, err := splitStudentBirth(values[6])
	if err != nil {
		return student, studentImportIssue{line, studentColumns[6], err.Error()}, nil
	}
	student.profile = studentProfile{
		NISN: values[1], Gender: strings.ToUpper(values[3]), ClassParallel: values[5],
		BirthPlace: place, BirthDate: date, NIK: values[7], HomeAddress: values[8],
	}
	if column, message := student.profile.normalize(); message != "" {
		return student, studentImportIssue{line, column, message}, nil
	}
	return student, studentImportIssue{}, nil
}

func splitStudentBirth(value string) (string, string, error) {
	if value == "" {
		return "", "", nil
	}
	place, date := "", value
	if before, after, found := strings.Cut(value, ","); found {
		place, date = strings.TrimSpace(before), strings.TrimSpace(after)
	} else if value[0] < '0' || value[0] > '9' {
		return value, "", nil
	}
	if date == "" {
		return place, "", nil
	}
	fields := strings.Fields(date)
	if len(fields) == 3 && len(fields[0]) <= 2 && len(fields[2]) == 4 &&
		decimalDigits.MatchString(fields[0]) && decimalDigits.MatchString(fields[2]) {
		months := []string{"januari", "februari", "maret", "april", "mei", "juni", "juli", "agustus", "september", "oktober", "november", "desember"}
		if month := slices.Index(months, strings.ToLower(fields[1])); month >= 0 {
			day, _ := strconv.Atoi(fields[0])
			date = fmt.Sprintf("%s-%02d-%02d", fields[2], month+1, day)
		}
	}
	parsed, err := time.Parse(time.DateOnly, date)
	if err != nil || parsed.Year() < 1 || parsed.Format(time.DateOnly) != date {
		return "", "", errors.New("Gunakan tanggal yang valid, misalnya Bogor, 15 Januari 2012.")
	}
	return place, date, nil
}

func studentTemplate() (*bytes.Buffer, error) {
	book := excelize.NewFile()
	defer book.Close()
	if err := book.SetSheetName("Sheet1", studentSheet); err != nil {
		return nil, err
	}
	if _, err := book.NewSheet("Contoh"); err != nil {
		return nil, err
	}
	textStyle, err := book.NewStyle(&excelize.Style{NumFmt: 49})
	if err != nil {
		return nil, err
	}
	headerStyle, err := book.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"216455"}},
	})
	if err != nil {
		return nil, err
	}
	for _, sheet := range []string{studentSheet, "Contoh"} {
		for _, col := range []string{"A", "B", "H"} {
			if err := book.SetColStyle(sheet, col, textStyle); err != nil {
				return nil, err
			}
		}
		widths := []float64{22, 14, 25, 8, 8, 19, 32, 22, 48}
		for i, title := range studentColumns {
			col, err := excelize.ColumnNumberToName(i + 1)
			if err != nil {
				return nil, err
			}
			if err := book.SetColWidth(sheet, col, col, widths[i]); err != nil {
				return nil, err
			}
			if err := book.SetCellStr(sheet, col+"1", title); err != nil {
				return nil, err
			}
		}
		if err := book.SetCellStyle(sheet, "A1", "I1", headerStyle); err != nil {
			return nil, err
		}
	}
	example := []any{"000000000000000018", "0000000007", "Alya Contoh", "P", "9", "3", "Bogor, 15 Januari 2012", "0000000000000016", "Jalan Contoh No. 12"}
	if err := book.SetSheetRow("Contoh", "A2", &example); err != nil {
		return nil, err
	}
	return book.WriteToBuffer()
}
