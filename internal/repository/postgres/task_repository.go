package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	taskdomain "example.com/taskservice/internal/domain/task"
	taskusecase "example.com/taskservice/internal/usecase/task"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
	const query = `
		INSERT INTO tasks (title, description, status, created_at, updated_at, start_at, deadline, master_task_id, schedule)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, title, description, status, created_at, updated_at, start_at, deadline, master_task_id, schedule
	`

	row := r.pool.QueryRow(ctx, query,
		task.Title,
		task.Description,
		task.Status,
		task.CreatedAt,
		task.UpdatedAt,
		task.StartAt,
		task.Deadline,
		task.MasterTaskId,
		scheduleDAOFromModel(task.Schedule),
	)
	created, err := scanTask(row)
	if err != nil {
		return nil, err
	}

	return created, nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*taskdomain.Task, error) {
	const query = `
		SELECT id, title, description, status, created_at, updated_at, start_at, deadline, master_task_id, schedule
		FROM tasks
		WHERE id = $1
	`

	row := r.pool.QueryRow(ctx, query, id)
	found, err := scanTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, taskdomain.ErrNotFound
		}

		return nil, err
	}

	return found, nil
}

func (r *Repository) Update(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
	const query = `
		UPDATE tasks
		SET title = $1,
			description = $2,
			status = $3,
			updated_at = $4,
			start_at = $5,
			deadline = $6,
			master_task_id = $7,
			schedule = $8
		WHERE id = $9
		RETURNING id, title, description, status, created_at, updated_at, start_at, deadline, master_task_id, schedule
	`

	row := r.pool.QueryRow(ctx, query,
		task.Title,
		task.Description,
		task.Status,
		task.UpdatedAt,
		task.StartAt,
		task.Deadline,
		task.MasterTaskId,
		scheduleDAOFromModel(task.Schedule),
		task.ID)
	updated, err := scanTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, taskdomain.ErrNotFound
		}

		return nil, err
	}

	return updated, nil
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	const query = `DELETE FROM tasks WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return taskdomain.ErrNotFound
	}

	return nil
}

func (r *Repository) List(ctx context.Context) ([]taskdomain.Task, error) {
	const query = `
		SELECT id, title, description, status, created_at, updated_at, start_at, deadline, master_task_id, schedule
		FROM tasks
		ORDER BY start_at DESC NULLS LAST, id DESC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]taskdomain.Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}

		tasks = append(tasks, *task)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

func (r *Repository) CreateBulk(ctx context.Context, tasks []taskdomain.Task) ([]taskdomain.Task, error) {
	if len(tasks) == 0 {
		return []taskdomain.Task{}, nil
	}

	const query = `
		INSERT INTO tasks (title, description, status, created_at, updated_at, start_at, deadline, master_task_id, schedule)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, title, description, status, created_at, updated_at, start_at, deadline, master_task_id, schedule
	`

	batch := &pgx.Batch{}
	for _, task := range tasks {
		batch.Queue(query,
			task.Title,
			task.Description,
			task.Status,
			task.CreatedAt,
			task.UpdatedAt,
			task.StartAt,
			task.Deadline,
			task.MasterTaskId,
			scheduleDAOFromModel(task.Schedule),
		)
	}

	br := r.pool.SendBatch(ctx, batch)

	createdTasks := make([]taskdomain.Task, 0, len(tasks))
	for i := 0; i < len(tasks); i++ {
		row := br.QueryRow()
		task, err := scanTask(row)
		if err != nil {
			br.Close()
			return nil, err
		}
		createdTasks = append(createdTasks, *task)
	}

	if err := br.Close(); err != nil {
		return nil, err
	}

	slices.SortFunc(createdTasks, func(a, b taskdomain.Task) int {
		if a.StartAt != nil && b.StartAt != nil {
			return -a.StartAt.Compare(*b.StartAt)
		}
		if a.StartAt != nil && b.StartAt == nil {
			return 1
		}
		if a.StartAt == nil && b.StartAt != nil {
			return -1
		}
		return int(b.ID) - int(a.ID)
	})

	return createdTasks, nil
}

func (r *Repository) UpdateTitleAndDescriptionByMasterID(ctx context.Context, task *taskdomain.Task, filter *taskusecase.Filter) error {
	query := `
        UPDATE tasks
        SET title = $1,
            description = $2,
            updated_at = $3
        WHERE master_task_id = $4
    `
	args := []any{task.Title, task.Description, task.UpdatedAt, task.ID}

	if filter != nil && filter.Status != nil && *filter.Status != taskdomain.Status("") {
		query = string(fmt.Appendf([]byte(query), " AND status = $%d", len(args)+1))
		args = append(args, filter.Status)
	}

	result, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return taskdomain.ErrNotFound
	}

	return nil
}

func (r *Repository) GetByMasterID(ctx context.Context, id int64) ([]taskdomain.Task, error) {
	const query = `
		SELECT id, title, description, status, created_at, updated_at, start_at, deadline, master_task_id, schedule
		FROM tasks
		WHERE master_task_id = $1 OR id = $1
		ORDER BY start_at DESC NULLS LAST, id DESC
	`

	rows, err := r.pool.Query(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]taskdomain.Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}

		tasks = append(tasks, *task)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

type taskScanner interface {
	Scan(dest ...any) error
}

func scanTask(scanner taskScanner) (*taskdomain.Task, error) {
	var (
		task         taskdomain.Task
		status       string
		start_at     *time.Time
		deadline     *time.Time
		masterTaskId *int64
		schedule     *scheduleDAO
	)

	if err := scanner.Scan(
		&task.ID,
		&task.Title,
		&task.Description,
		&status,
		&task.CreatedAt,
		&task.UpdatedAt,
		&start_at,
		&deadline,
		&masterTaskId,
		&schedule,
	); err != nil {
		return nil, err
	}

	task.Status = taskdomain.Status(status)
	if start_at != nil {
		task.StartAt = start_at
	}
	if deadline != nil {
		task.Deadline = deadline
	}
	if masterTaskId != nil {
		task.MasterTaskId = masterTaskId
	}
	if schedule != nil {
		scheduleModel := scheduleModelFromDAO(schedule)
		task.Schedule = scheduleModel
	}

	return &task, nil
}

type scheduleDAO struct {
	SparseDates      []time.Time `json:"sparse_dates,omitempty"`
	EveryNthDay      *int64      `json:"every_nth_day,omitempty"`
	EveryNthMonthDay *int64      `json:"every_nth_monthday,omitempty"`
	EveryEvenDay     *bool       `json:"every_even_day,omitempty"`
}

func scheduleModelFromDAO(dao *scheduleDAO) *taskdomain.Schedule {
	if dao == nil {
		return nil
	}
	scheduleModel := taskdomain.Schedule{}
	switch {
	case dao.EveryEvenDay != nil:
		scheduleModel.EveryEvenDay = dao.EveryEvenDay
	case dao.EveryNthDay != nil:
		scheduleModel.EveryNthDay = dao.EveryNthDay
	case dao.EveryNthMonthDay != nil:
		scheduleModel.EveryNthMonthDay = dao.EveryNthMonthDay
	case len(dao.SparseDates) > 0:
		scheduleModel.SparseDates = dao.SparseDates
	}
	return &scheduleModel
}

func scheduleDAOFromModel(model *taskdomain.Schedule) *scheduleDAO {
	if model == nil {
		return nil
	}
	dao := scheduleDAO{}
	switch {
	case model.EveryEvenDay != nil:
		dao.EveryEvenDay = model.EveryEvenDay
	case model.EveryNthDay != nil:
		dao.EveryNthDay = model.EveryNthDay
	case model.EveryNthMonthDay != nil:
		dao.EveryNthMonthDay = model.EveryNthMonthDay
	case len(model.SparseDates) > 0:
		dao.SparseDates = model.SparseDates
	}
	return &dao
}
