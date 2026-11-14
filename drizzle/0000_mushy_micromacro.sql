CREATE TABLE `attendance` (
	`id` text PRIMARY KEY NOT NULL,
	`person_id` text NOT NULL,
	`day` text NOT NULL,
	`arrived` text NOT NULL,
	`departed` text,
	`status` text NOT NULL,
	`late_minutes` integer DEFAULT 0 NOT NULL,
	FOREIGN KEY (`person_id`) REFERENCES `people`(`id`) ON UPDATE no action ON DELETE no action
);
--> statement-breakpoint
CREATE UNIQUE INDEX `attendance_person_day` ON `attendance` (`person_id`,`day`);--> statement-breakpoint
CREATE TABLE `people` (
	`id` text PRIMARY KEY NOT NULL,
	`token` text NOT NULL,
	`name` text NOT NULL,
	`number` text NOT NULL,
	`role` text NOT NULL,
	`group_name` text NOT NULL,
	`active` integer DEFAULT 1 NOT NULL,
	`demo` integer DEFAULT 0 NOT NULL
);
--> statement-breakpoint
CREATE UNIQUE INDEX `people_token_unique` ON `people` (`token`);--> statement-breakpoint
CREATE UNIQUE INDEX `people_number_unique` ON `people` (`number`);--> statement-breakpoint
CREATE TABLE `settings` (
	`id` text PRIMARY KEY NOT NULL,
	`school` text NOT NULL,
	`student_time` text NOT NULL,
	`teacher_time` text NOT NULL,
	`timezone` text NOT NULL
);
