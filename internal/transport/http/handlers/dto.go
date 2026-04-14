package handlers

import (
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

type taskMutationDTO struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      taskdomain.Status `json:"status"`

	StartAt  *time.Time `json:"start_at,omitempty"`
	Deadline *time.Time `json:"deadline,omitempty"`

	MakeStandalone  bool                        `json:"make_standalone,omitempty"`
	ChainGeneration *chainGenerationMutationDTO `json:"chain_generation,omitempty"`
}

type taskDTO struct {
	ID          int64             `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      taskdomain.Status `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`

	StartAt  *time.Time `json:"start_at,omitempty"`
	Deadline *time.Time `json:"deadline,omitempty"`

	MasterTaskId *int64       `json:"master_task_id,omitempty"`
	Schedule     *scheduleDTO `json:"schedule,omitempty"`
}

type chainGenerationMutationDTO struct {
	Schedule scheduleDTO `json:"schedule"`
	Start    *time.Time  `json:"start,omitempty"`
	End      *time.Time  `json:"end,omitempty"`
}

type chainPropagationMutationDTO struct {
	Start *time.Time `json:"start,omitempty"`
	End   *time.Time `json:"end,omitempty"`
}

type scheduleDTO struct {
	// SparseDates создать задачи только в указанные даты
	SparseDates []time.Time `json:"sparse_dates,omitempty"`
	// EveryNthDay создать задачу в каждый N-й день
	EveryNthDay *int64 `json:"every_nth_day,omitempty"`
	// EveryNthMonthDay создавать задачу в N-ое число месяца
	EveryNthMonthDay *int64 `json:"every_nth_monthday,omitempty"`
	// EveryEvenDay создавать задачи в четные числа (true) или нечетные (false)
	EveryEvenDay *bool `json:"every_even_day,omitempty"`
}

func newTaskDTO(task *taskdomain.Task) taskDTO {
	dto := taskDTO{
		ID:          task.ID,
		Title:       task.Title,
		Description: task.Description,
		Status:      task.Status,
		CreatedAt:   task.CreatedAt,
		UpdatedAt:   task.UpdatedAt,
	}
	if task.StartAt != nil {
		dto.StartAt = task.StartAt
	}
	if task.Deadline != nil {
		dto.Deadline = task.Deadline
	}
	if task.MasterTaskId != nil {
		dto.MasterTaskId = task.MasterTaskId
	}
	if task.Schedule != nil {
		schedule := scheduleDTO{}
		switch {
		case task.Schedule.EveryEvenDay != nil:
			schedule.EveryEvenDay = task.Schedule.EveryEvenDay
		case task.Schedule.EveryNthDay != nil:
			schedule.EveryNthDay = task.Schedule.EveryNthDay
		case task.Schedule.EveryNthMonthDay != nil:
			schedule.EveryNthMonthDay = task.Schedule.EveryNthMonthDay
		case len(task.Schedule.SparseDates) > 0:
			schedule.SparseDates = task.Schedule.SparseDates
		}
		dto.Schedule = &schedule
	}
	return dto
}
