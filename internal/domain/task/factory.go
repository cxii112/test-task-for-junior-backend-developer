package task

import (
	"time"
)

// WorkdayChecker функция для проверки даты на рабочесть
type WorkdayChecker func(time.Time) bool

func DefaultWorkdayChecker(day time.Time) bool {
	return !(day.Weekday() == time.Saturday || day.Weekday() == time.Sunday)
}

type GenerationOptions struct {
	WorkdayChecker  WorkdayChecker
	GenerationStart *time.Time
	GenerationEnd   *time.Time
	Now             time.Time
}

// NewChainFromMaster генерирует задачи по мастер-задаче.
// Если [Schedule].SparseDates не nil, то время на ограничения генерации игнорируется,
// проверка на рабочий день не осуществляется.
// Во всех остальных ситуациях день проверяется на рабочий день,
// границы учитываются
func NewChainFromMaster(
	master Task,
	schedule Schedule,
	opts *GenerationOptions,
) []Task {
	if opts == nil {
		opts = GetDefaultGenerationOpts()
	}
	tasks := []Task{}
	if len(schedule.SparseDates) > 0 {
		return newChainForSparseDates(master, schedule.SparseDates, opts)
	}
	if schedule.EveryNthDay != nil {
		return newChainForEveryNthDay(master, *schedule.EveryNthDay, opts)
	}
	if schedule.EveryNthMonthDay != nil {
		return newChainForEveryNthMonthDay(master, *schedule.EveryNthMonthDay, opts)
	}
	if schedule.EveryEvenDay != nil {
		return newChainForEveryEvenDay(master, *schedule.EveryEvenDay, opts)
	}
	return tasks
}

// NewSingle создает одиночную задачу.
// Предполагает, что startAt и deadline валидны.
// Время на выполнения задачи будет вычислено из разницы deadline и startAt.
// Проверка на рабочий день не осуществляется
func NewSingle(
	title string,
	description string,
	status Status,
	startAt, deadline *time.Time,
	opts *GenerationOptions,
) Task {
	if opts == nil {
		opts = GetDefaultGenerationOpts()
	}
	now := opts.Now
	t := Task{
		Title:       title,
		Description: description,
		Status:      status,
		CreatedAt:   now,
		UpdatedAt:   now,
		StartAt:     &now,
	}
	if startAt != nil {
		t.StartAt = startAt
	}
	if deadline != nil {
		t.Deadline = deadline
	}
	return t
}

// newChainForSparseDates генерирует цепочку задач в указанные даты из мастер-задачи.
// Проверка на рабочий день не осуществляется
func newChainForSparseDates(
	master Task,
	dates []time.Time,
	opts *GenerationOptions,
) []Task {
	if opts == nil {
		opts = GetDefaultGenerationOpts()
	}
	tasks := []Task{}
	var window *time.Duration
	if master.Deadline != nil && master.StartAt != nil {
		w := master.Deadline.Sub(*master.StartAt)
		window = &w
	}
	for _, date := range dates {
		var newDeadline *time.Time
		if window != nil {
			d := date.Add(*window)
			newDeadline = &d
		}
		task := NewSingle(
			master.Title,
			master.Description,
			master.Status,
			&date,
			newDeadline,
			opts,
		)
		task.MasterTaskId = &master.ID
		tasks = append(tasks, task)
	}
	return tasks
}

// newChainForEveryNthDay генерирует цепочку задач на каждый N-й день из мастер-задачи.
// Добавляет задачи только в рабочие дни
func newChainForEveryNthDay(
	master Task,
	gap int64,
	opts *GenerationOptions,
) []Task {
	if opts == nil {
		opts = GetDefaultGenerationOpts()
	}
	if opts.GenerationStart == nil {
		opts.GenerationStart = &opts.Now
	}
	if opts.GenerationEnd == nil {
		end := opts.GenerationStart.Add(DefaultTasksGenerationWindow)
		opts.GenerationEnd = &end
	}
	tasks := []Task{}
	var window *time.Duration
	if master.Deadline != nil && master.StartAt != nil {
		w := master.Deadline.Sub(*master.StartAt)
		window = &w
	}
	current := *opts.GenerationStart
	next := func() {
		current = current.AddDate(0, 0, int(gap))
	}
	for current.Before(*opts.GenerationEnd) {
		if !opts.WorkdayChecker(current) {
			next()
			continue
		}
		if master.StartAt != nil && current.Before(*master.StartAt) {
			next()
			continue
		}
		if master.StartAt != nil && master.StartAt.Sub(current).Abs() < time.Second {
			next()
			continue
		}
		startDate := current
		var newDeadline *time.Time
		if window != nil {
			d := startDate.Add(*window)
			newDeadline = &d
		}
		task := NewSingle(
			master.Title,
			master.Description,
			master.Status,
			&startDate,
			newDeadline,
			opts,
		)
		task.MasterTaskId = &master.ID
		tasks = append(tasks, task)
		current = current.AddDate(0, 0, int(gap))
	}
	return tasks
}

// newChainForEveryNthMonthDay генерирует цепочку задач на N-й день месяца из мастер-задачи.
// Добавляет задачи только в рабочие дни
func newChainForEveryNthMonthDay(
	master Task,
	monthDay int64,
	opts *GenerationOptions,
) []Task {
	if opts == nil {
		opts = GetDefaultGenerationOpts()
	}
	if opts.GenerationStart == nil {
		opts.GenerationStart = &opts.Now
	}
	if opts.GenerationEnd == nil {
		end := opts.GenerationStart.Add(DefaultTasksGenerationWindow)
		opts.GenerationEnd = &end
	}
	tasks := []Task{}
	var window *time.Duration
	if master.Deadline != nil && master.StartAt != nil {
		w := master.Deadline.Sub(*master.StartAt)
		window = &w
	}

	startYear := opts.GenerationStart.Year()
	startMonth := opts.GenerationStart.Month()
	targetHour := master.StartAt.Hour()
	targetMinute := master.StartAt.Minute()
	targetSecond := master.StartAt.Second()
	daysToAdd := func(month time.Month) int {
		switch month {
		case time.January, time.March, time.May, time.July, time.August, time.October, time.December:
			return 31
		case time.April, time.June, time.September, time.November:
			return 30
		case time.February:
			return 29
		default:
			return 29
		}
	}

	current := time.Date(
		startYear,
		startMonth,
		int(monthDay),
		targetHour, targetMinute, targetSecond, 0,
		time.UTC,
	)
	next := func() {
		month := current.Month()
		current = current.AddDate(0, 0, daysToAdd(month))
		if current.Month()-1 > month {
			current = current.AddDate(0, 0, -current.Day())
			return
		}
		if current.Day() != int(monthDay) && month == current.Month()-1 {
			current = current.AddDate(0, 0, int(monthDay)-current.Day())
		}
	}

	for current.Before(*opts.GenerationEnd) {
		if current.Before(*opts.GenerationStart) {
			next()
			continue
		}
		if master.StartAt != nil && current.Before(*master.StartAt) {
			next()
			continue
		}
		if master.StartAt != nil && master.StartAt.Sub(current).Abs() < time.Second {
			next()
			continue
		}
		if !opts.WorkdayChecker(current) {
			next()
			continue
		}
		startDate := current
		var newDeadline *time.Time
		if window != nil {
			d := startDate.Add(*window)
			newDeadline = &d
		}
		task := NewSingle(
			master.Title,
			master.Description,
			master.Status,
			&startDate,
			newDeadline,
			opts,
		)
		task.MasterTaskId = &master.ID
		tasks = append(tasks, task)
		next()
	}
	return tasks
}

// newChainForEveryEvenDay генеррирует цепочку задач на четные/нечетные дни из мастер-задачи.
// Добавляет задачи только в рабочие дни
func newChainForEveryEvenDay(
	master Task,
	choseEven bool,
	opts *GenerationOptions,
) []Task {
	if opts == nil {
		opts = GetDefaultGenerationOpts()
	}
	if opts.GenerationStart == nil {
		opts.GenerationStart = &opts.Now
	}
	if opts.GenerationEnd == nil {
		end := opts.GenerationStart.Add(DefaultTasksGenerationWindow)
		opts.GenerationEnd = &end
	}

	var monthStartDay int
	if choseEven {
		monthStartDay = 2
	} else {
		monthStartDay = 1
	}
	startYear := opts.GenerationStart.Year()
	startMonth := opts.GenerationStart.Month()
	targetHour := master.StartAt.Hour()
	targetMinute := master.StartAt.Minute()
	targetSecond := master.StartAt.Second()

	current := time.Date(
		startYear,
		startMonth,
		monthStartDay,
		targetHour, targetMinute, targetSecond, 0,
		time.UTC,
	)

	next := func() {
		month := current.Month()
		current = current.AddDate(0, 0, 2)
		if current.Month() != month && current.Day() != monthStartDay {
			current = current.AddDate(0, 0, monthStartDay-current.Day())
		}
	}
	tasks := []Task{}
	var window *time.Duration
	if master.Deadline != nil && master.StartAt != nil {
		w := master.Deadline.Sub(*master.StartAt)
		window = &w
	}
	for current.Before(*opts.GenerationEnd) {
		if current.Before(*opts.GenerationStart) {
			next()
			continue
		}
		if master.StartAt != nil && current.Before(*master.StartAt) {
			next()
			continue
		}
		if master.StartAt != nil && master.StartAt.Sub(current).Abs() < time.Second {
			next()
			continue
		}
		if !opts.WorkdayChecker(current) {
			next()
			continue
		}
		startDate := current
		var newDeadline *time.Time
		if window != nil {
			d := startDate.Add(*window)
			newDeadline = &d
		}
		task := NewSingle(
			master.Title,
			master.Description,
			master.Status,
			&startDate,
			newDeadline,
			opts,
		)
		task.MasterTaskId = &master.ID
		tasks = append(tasks, task)
		next()
	}
	return tasks
}

func GetDefaultGenerationOpts() *GenerationOptions {
	return &GenerationOptions{
		Now:            time.Now().UTC(),
		WorkdayChecker: DefaultWorkdayChecker,
	}
}

// DefaultTasksGenerationWindow окно генераации задач по расписанию.
// Примерно равно 3 месяцам (24 * 30 * 3 часов)
const DefaultTasksGenerationWindow = time.Hour * 24 * 30 * 3
