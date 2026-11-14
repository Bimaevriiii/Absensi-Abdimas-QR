import { integer, sqliteTable, text, uniqueIndex } from 'drizzle-orm/sqlite-core';
export const people = sqliteTable('people', {
 id:text('id').primaryKey(), token:text('token').notNull().unique(), name:text('name').notNull(), number:text('number').notNull().unique(), role:text('role').notNull(), group:text('group_name').notNull(), active:integer('active').notNull().default(1), demo:integer('demo').notNull().default(0)
});
export const attendance = sqliteTable('attendance', {
 id:text('id').primaryKey(), personId:text('person_id').notNull().references(()=>people.id), day:text('day').notNull(), arrived:text('arrived').notNull(), departed:text('departed'), status:text('status').notNull(), lateMinutes:integer('late_minutes').notNull().default(0)
}, t=>[uniqueIndex('attendance_person_day').on(t.personId,t.day)]);
export const settings = sqliteTable('settings', {id:text('id').primaryKey(), school:text('school').notNull(), studentTime:text('student_time').notNull(), teacherTime:text('teacher_time').notNull(), timezone:text('timezone').notNull()});
