CREATE TABLE IF NOT EXISTS people (
    id text PRIMARY KEY,
    token text NOT NULL UNIQUE,
    name text NOT NULL,
    number text NOT NULL UNIQUE,
    role text NOT NULL CHECK (role IN ('murid', 'guru')),
    group_name text NOT NULL,
    active boolean NOT NULL DEFAULT true,
    demo boolean NOT NULL DEFAULT false
);

CREATE TABLE IF NOT EXISTS attendance (
    id text PRIMARY KEY,
    person_id text NOT NULL REFERENCES people(id),
    day date NOT NULL,
    arrived timestamptz NOT NULL,
    departed timestamptz,
    status text NOT NULL CHECK (status IN ('Tepat waktu', 'Terlambat')),
    late_minutes integer NOT NULL DEFAULT 0 CHECK (late_minutes >= 0),
    UNIQUE (person_id, day)
);
CREATE INDEX IF NOT EXISTS attendance_day_idx ON attendance(day);

CREATE TABLE IF NOT EXISTS settings (
    id text PRIMARY KEY CHECK (id = 'main'),
    school text NOT NULL,
    student_time text NOT NULL,
    teacher_time text NOT NULL,
    timezone text NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
    id text PRIMARY KEY,
    email text NOT NULL UNIQUE,
    name text NOT NULL,
    role text NOT NULL CHECK (role IN ('pengelola', 'operator', 'petugas')),
    password_hash text NOT NULL,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sessions (
    token_hash text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sessions_expires_at_idx ON sessions(expires_at);
