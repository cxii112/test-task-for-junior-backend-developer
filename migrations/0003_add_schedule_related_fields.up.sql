UPDATE tasks
    SET start_at = created_at
    WHERE start_at IS NULL;
ALTER TABLE tasks
    ADD COLUMN master_task_id BIGINT,
    ADD CONSTRAINT fk_tasks_master_task FOREIGN KEY (master_task_id) REFERENCES tasks(id) ON DELETE SET NULL;
ALTER TABLE tasks
    ADD COLUMN schedule JSONB;
