package task

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*taskdomain.Task, error) {
	normalized, err := validateCreateInput(input)
	if err != nil {
		return nil, err
	}

	now := s.now()

	var schedule *taskdomain.Schedule
	var opts *taskdomain.GenerationOptions
	if normalized.Generation != nil {
		schedule = &taskdomain.Schedule{}
		switch {
		case normalized.Generation.Schedule.SparseDates != nil:
			schedule.SparseDates = normalized.Generation.Schedule.SparseDates
		case normalized.Generation.Schedule.EveryEvenDay != nil:
			schedule.EveryEvenDay = normalized.Generation.Schedule.EveryEvenDay
		case normalized.Generation.Schedule.EveryNthDay != nil:
			schedule.EveryNthDay = normalized.Generation.Schedule.EveryNthDay
		case normalized.Generation.Schedule.EveryNthMonthDay != nil:
			schedule.EveryNthMonthDay = normalized.Generation.Schedule.EveryNthMonthDay
		}
		opts = &taskdomain.GenerationOptions{
			Now:            now,
			WorkdayChecker: taskdomain.DefaultWorkdayChecker,
		}
		if normalized.Generation.Start != nil {
			opts.GenerationStart = normalized.Generation.Start
		} else {
			opts.GenerationStart = &now
		}
		if normalized.Generation.End != nil {
			opts.GenerationEnd = normalized.Generation.End
		}
	}

	model := taskdomain.NewSingle(
		normalized.Title,
		normalized.Description,
		normalized.Status,
		normalized.StartAt,
		normalized.Deadline,
		&taskdomain.GenerationOptions{
			Now:            now,
			WorkdayChecker: taskdomain.DefaultWorkdayChecker,
		},
	)

	if schedule == nil {
		return s.repo.Create(ctx, &model)
	}
	model.Schedule = schedule

	if len(normalized.Generation.Schedule.SparseDates) > 0 {
		chain := taskdomain.NewChainFromMaster(
			model,
			*schedule,
			opts,
		)
		bulk, err := s.repo.CreateBulk(ctx, chain)
		if err != nil {
			return nil, err
		}
		return &bulk[0], nil
	}

	created, err := s.repo.Create(ctx, &model)
	if err != nil {
		return nil, err
	}

	chain := taskdomain.NewChainFromMaster(
		*created,
		*schedule,
		opts,
	)
	_, err = s.repo.CreateBulk(ctx, chain)
	if err != nil {
		return nil, err
	}

	return created, nil
}

func (s *Service) GetByID(ctx context.Context, id int64) (*taskdomain.Task, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	return s.repo.GetByID(ctx, id)
}

func (s *Service) Update(ctx context.Context, id int64, input UpdateInput) (*taskdomain.Task, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	target, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	normalized, err := validateUpdateInput(input)
	if err != nil {
		return nil, err
	}

	model := &taskdomain.Task{
		ID:          id,
		Title:       normalized.Title,
		Description: normalized.Description,
		Status:      normalized.Status,
		UpdatedAt:   s.now(),
	}
	if target.Schedule != nil {
		model.Schedule = target.Schedule
	}
	if target.MasterTaskId != nil {
		model.MasterTaskId = target.MasterTaskId
	}
	if target.Schedule != nil {
		model.Schedule = target.Schedule
	}
	if normalized.MakeStandalone {
		model.MasterTaskId = nil
	}

	if normalized.StartAt == nil {
		// для старых клиентов
		model.StartAt = target.StartAt
	} else {
		model.StartAt = normalized.StartAt
	}
	if normalized.Deadline == nil {
		// для старых клиентов
		model.Deadline = target.Deadline
	} else {
		model.Deadline = normalized.Deadline
	}

	if normalized.StartAt != nil && target.StartAt != nil &&
		normalized.StartAt.Sub(*target.StartAt).Abs() > time.Second {
		model.MasterTaskId = nil

	}
	if normalized.Deadline != nil && target.Deadline != nil &&
		normalized.Deadline.Sub(*target.Deadline).Abs() > time.Second {
		model.MasterTaskId = nil

	}

	updated, err := s.repo.Update(ctx, model)
	if err != nil {
		return nil, err
	}

	chain, err := s.repo.GetByMasterID(ctx, id)
	if err != nil {
		slog.WarnContext(ctx, "error during updating dependent tasks", "error", err.Error(), "master_id", id)
	}
	if len(chain) <= 1 {
		return updated, nil
	}

	status := taskdomain.StatusNew
	err = s.repo.UpdateTitleAndDescriptionByMasterID(ctx, model, &Filter{
		Status: &status,
	})
	if err != nil {
		slog.WarnContext(ctx, "error during updating dependent tasks", "error", err.Error(), "master_id", id)
	}

	return updated, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	return s.repo.Delete(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]taskdomain.Task, error) {
	return s.repo.List(ctx)
}

func (s *Service) GetChainByMasterID(ctx context.Context, id int64) ([]taskdomain.Task, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}
	return s.repo.GetByMasterID(ctx, id)
}

func validateCreateInput(input CreateInput) (CreateInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)

	if input.Title == "" {
		return CreateInput{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}

	if input.Status == "" {
		input.Status = taskdomain.StatusNew
	}

	if !input.Status.Valid() {
		return CreateInput{}, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}

	if input.StartAt == nil {
		input.Deadline = nil
		return input, nil
	}

	if input.Deadline == nil {
		return input, nil
	}

	startAt := input.StartAt.UTC()
	deadline := input.Deadline.UTC()
	if startAt.After(deadline) {
		return CreateInput{}, fmt.Errorf("%w: deadline must be after start time", ErrInvalidInput)
	}
	input.StartAt = &startAt
	input.Deadline = &deadline

	if input.Generation == nil {
		return input, nil
	}

	if input.Generation.End != nil && input.Generation.Start != nil &&
		input.Generation.End.Before(*input.Generation.Start) {
		return CreateInput{}, fmt.Errorf("%w: generation end time must be after start time", ErrInvalidInput)
	}
	if input.Generation.Start != nil {
		s := input.Generation.Start.UTC()
		input.Generation.Start = &s
	}
	if input.Generation.End != nil {
		e := input.Generation.End.UTC()
		input.Generation.End = &e
	}

	fieldsCount := 0
	if len(input.Generation.Schedule.SparseDates) > 0 {
		fieldsCount += 1
	}
	if input.Generation.Schedule.EveryEvenDay != nil {
		fieldsCount += 1
	}
	if input.Generation.Schedule.EveryNthDay != nil {
		fieldsCount += 1
	}
	if input.Generation.Schedule.EveryNthMonthDay != nil {
		fieldsCount += 1
	}
	if fieldsCount > 1 {
		return CreateInput{}, fmt.Errorf("%w: schedule input field are mutualy exclusive", ErrInvalidInput)
	}
	switch {
	case input.Generation.Schedule.SparseDates != nil:
		dates := map[int64]struct{}{}
		for _, date := range input.Generation.Schedule.SparseDates {
			unixDate := date.Unix()
			_, exists := dates[unixDate]
			if exists {
				return CreateInput{}, fmt.Errorf("%w: scheduled sparse dates must be unique", ErrInvalidInput)
			}
			dates[unixDate] = struct{}{}
		}
	case input.Generation.Schedule.EveryNthMonthDay != nil:
		if *input.Generation.Schedule.EveryNthMonthDay > 31 {
			return CreateInput{}, fmt.Errorf("%w: scheduled every Nth month day must be in range from 1 to 31 inclusive", ErrInvalidInput)
		}
	}

	return input, nil
}

func (s *Service) CreateChainFromTask(ctx context.Context, id int64, input GenerationInput) ([]taskdomain.Task, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}
	normalized, err := validateCreateChainInput(input)
	if err != nil {
		return nil, err
	}
	master, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	oldChain, err := s.repo.GetByMasterID(ctx, master.ID)
	if err != nil {
		return nil, err
	}
	if len(oldChain) > 1 {
		return nil, fmt.Errorf("%w: all tasks must be unlinked from current task", ErrInvalidInput)
	}
	if len(oldChain) == 1 && oldChain[0].ID != id {
		return nil, fmt.Errorf("%w: all tasks must be unlinked from current task", ErrInvalidInput)
	}
	now := s.now()
	if master.MasterTaskId != nil {
		master.MasterTaskId = nil
	}
	schedule := taskdomain.Schedule{}
	switch {
	case normalized.Schedule.SparseDates != nil:
		schedule.SparseDates = normalized.Schedule.SparseDates
	case normalized.Schedule.EveryEvenDay != nil:
		schedule.EveryEvenDay = normalized.Schedule.EveryEvenDay
	case normalized.Schedule.EveryNthDay != nil:
		schedule.EveryNthDay = normalized.Schedule.EveryNthDay
	case normalized.Schedule.EveryNthMonthDay != nil:
		schedule.EveryNthMonthDay = normalized.Schedule.EveryNthMonthDay
	}
	opts := &taskdomain.GenerationOptions{
		Now:            now,
		WorkdayChecker: taskdomain.DefaultWorkdayChecker,
	}
	if normalized.Start != nil {
		opts.GenerationStart = normalized.Start
	} else {
		opts.GenerationStart = &now
	}
	if normalized.End != nil {
		opts.GenerationEnd = normalized.End
	}
	master.Schedule = &schedule
	chain := taskdomain.NewChainFromMaster(*master, schedule, opts)
	bulk, err := s.repo.CreateBulk(ctx, chain)
	if err != nil {
		return nil, err
	}
	updated, err := s.repo.Update(ctx, master)
	if err != nil {
		return nil, err
	}
	extended := make([]taskdomain.Task, len(bulk))
	copy(extended, bulk)
	extended = append(extended, *updated)
	return extended, nil
}

func (s *Service) PropagateChain(ctx context.Context, id int64, input PropagationInput) ([]taskdomain.Task, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}
	target, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if target.Schedule == nil {
		return nil, fmt.Errorf("%w: can't propagate unscheduled task", ErrInvalidInput)
	}
	normalized, err := validatePropagateChainInput(input)
	if err != nil {
		return nil, err
	}
	chain, err := s.repo.GetByMasterID(ctx, id)
	if len(chain) < 1 {
		return nil, fmt.Errorf("%w: can't propagate empty chain", ErrInvalidInput)
	}
	var latestDate time.Time
	now := s.now()
	if normalized.Start != nil {
		latestDate = *normalized.Start
	}
	for _, task := range chain {
		if task.ID == id {
			continue
		}
		if task.StartAt == nil {
			continue
		}
		if !task.StartAt.After(latestDate) {
			continue
		}
		latestDate = *task.StartAt
	}

	opts := &taskdomain.GenerationOptions{
		Now:             now,
		WorkdayChecker:  taskdomain.DefaultWorkdayChecker,
		GenerationStart: &latestDate,
	}
	if normalized.End != nil {
		opts.GenerationEnd = normalized.End
	}

	newChain := taskdomain.NewChainFromMaster(
		*target,
		*target.Schedule,
		opts,
	)
	added, err := s.repo.CreateBulk(ctx, newChain)
	if err != nil {
		return nil, err
	}
	chain = append(chain, added...)
	return chain, nil
}

func validateUpdateInput(input UpdateInput) (UpdateInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)

	if input.Title == "" {
		return UpdateInput{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}

	if !input.Status.Valid() {
		return UpdateInput{}, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}

	if input.StartAt == nil {
		input.Deadline = nil
		return input, nil
	}
	startAt := input.StartAt.UTC()
	input.StartAt = &startAt

	if input.Deadline == nil {
		return input, nil
	}

	deadline := input.Deadline.UTC()
	if startAt.After(deadline) {
		return UpdateInput{}, fmt.Errorf("%w: deadline must be after start time", ErrInvalidInput)
	}
	input.StartAt = &startAt
	input.Deadline = &deadline
	return input, nil
}

func validateCreateChainInput(input GenerationInput) (GenerationInput, error) {
	if input.End != nil && input.Start != nil &&
		input.End.Before(*input.Start) {
		return GenerationInput{}, fmt.Errorf("%w: deadline must be after start", ErrInvalidInput)
	}
	if input.Start != nil {
		s := input.Start.UTC()
		input.Start = &s
	}
	if input.End != nil {
		e := input.End.UTC()
		input.End = &e
	}

	fieldsCount := 0
	if len(input.Schedule.SparseDates) > 0 {
		fieldsCount += 1
	}
	if input.Schedule.EveryEvenDay != nil {
		fieldsCount += 1
	}
	if input.Schedule.EveryNthDay != nil {
		fieldsCount += 1
	}
	if input.Schedule.EveryNthMonthDay != nil {
		fieldsCount += 1
	}
	if fieldsCount > 1 {
		return GenerationInput{}, fmt.Errorf("%w: schedule input field are mutualy exclusive", ErrInvalidInput)
	}
	switch {
	case input.Schedule.SparseDates != nil:
		dates := map[int64]struct{}{}
		for _, date := range input.Schedule.SparseDates {
			unixDate := date.Unix()
			_, exists := dates[unixDate]
			if exists {
				return GenerationInput{}, fmt.Errorf("%w: scheduled sparse dates must be unique", ErrInvalidInput)
			}
			dates[unixDate] = struct{}{}
		}
	case input.Schedule.EveryNthMonthDay != nil:
		if *input.Schedule.EveryNthMonthDay > 31 {
			return GenerationInput{}, fmt.Errorf("%w: scheduled every Nth month day must be in range from 1 to 31 inclusive", ErrInvalidInput)
		}
	}

	return input, nil
}

func validatePropagateChainInput(input PropagationInput) (PropagationInput, error) {
	if input.End != nil && input.Start != nil &&
		input.End.Before(*input.Start) {
		return PropagationInput{}, fmt.Errorf("%w: deadline must be after start", ErrInvalidInput)
	}
	if input.Start != nil {
		s := input.Start.UTC()
		input.Start = &s
	}
	if input.End != nil {
		e := input.End.UTC()
		input.End = &e
	}

	return input, nil
}
