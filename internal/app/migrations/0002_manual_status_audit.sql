ALTER TABLE attendance ALTER COLUMN arrived DROP NOT NULL;
ALTER TABLE attendance ADD COLUMN note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 500);
ALTER TABLE attendance ADD COLUMN source text NOT NULL DEFAULT 'scan' CHECK (source IN ('scan', 'manual', 'correction'));
ALTER TABLE attendance DROP CONSTRAINT IF EXISTS attendance_status_check;
ALTER TABLE attendance ADD CONSTRAINT attendance_status_check
    CHECK (status IN ('Tepat waktu', 'Terlambat', 'Izin', 'Sakit', 'Alpa'));
ALTER TABLE attendance ADD CONSTRAINT attendance_consistency_check CHECK (
    (status IN ('Tepat waktu', 'Terlambat') AND arrived IS NOT NULL AND (departed IS NULL OR departed >= arrived))
    OR (status IN ('Izin', 'Sakit', 'Alpa') AND arrived IS NULL AND departed IS NULL AND late_minutes = 0)
);

ALTER TABLE settings ADD COLUMN student_departure_time text NOT NULL DEFAULT '13:00';
ALTER TABLE settings ADD COLUMN teacher_departure_time text NOT NULL DEFAULT '14:00';

CREATE TABLE attendance_audit (
    id text PRIMARY KEY,
    attendance_id text NOT NULL,
    person_id text NOT NULL,
    actor_user_id text NOT NULL REFERENCES users(id),
    action text NOT NULL CHECK (action IN ('manual_create', 'correction')),
    before_data jsonb,
    after_data jsonb NOT NULL,
    reason text NOT NULL CHECK (char_length(reason) BETWEEN 5 AND 500),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX attendance_audit_record_idx ON attendance_audit(attendance_id, created_at DESC);
CREATE INDEX attendance_audit_created_idx ON attendance_audit(created_at DESC);
