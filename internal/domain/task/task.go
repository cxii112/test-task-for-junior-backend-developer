package task

import "time"

type Status string

const (
	StatusNew        Status = "new"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

type Task struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// StartAt -- время начала задания. Если не установлено -- совпадает с CreatedAt
	StartAt *time.Time `json:"start_at"`
	// Deadline -- время до которого необходимо закончить задачу
	Deadline *time.Time `json:"deadline"`

	// Schedule расписание для создания задач из мастер-задачи.
	// Расписание изменяется только на мастер-задаче
	Schedule *Schedule
	// MasterTaskId идентификатор мастер-задачи.
	// Если nil -- задача является мастер-задачей
	MasterTaskId *int64
}

func (s Status) Valid() bool {
	switch s {
	case StatusNew, StatusInProgress, StatusDone:
		return true
	default:
		return false
	}
}
