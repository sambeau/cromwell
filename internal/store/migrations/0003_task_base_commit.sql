-- 0003_task_base_commit.sql — record each task's branch base so code review
-- sees the task's whole contribution, not just its last commit (a task may
-- span several implement dispatches on rework, DESIGN-006 §5). Forward-only.

ALTER TABLE tasks ADD COLUMN base_commit text;
