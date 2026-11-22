ALTER TABLE people
    ADD COLUMN nisn text NOT NULL DEFAULT '' CHECK (nisn = '' OR nisn ~ '^[0-9]{10}$'),
    ADD COLUMN gender text NOT NULL DEFAULT '' CHECK (gender IN ('', 'L', 'P')),
    ADD COLUMN class_parallel text NOT NULL DEFAULT '',
    ADD COLUMN birth_place text NOT NULL DEFAULT '',
    ADD COLUMN birth_date date,
    ADD COLUMN nik text NOT NULL DEFAULT '' CHECK (nik = '' OR nik ~ '^[0-9]{16}$'),
    ADD COLUMN home_address text NOT NULL DEFAULT '';

CREATE UNIQUE INDEX people_student_nisn_idx ON people (nisn)
    WHERE role = 'murid' AND nisn <> '';
