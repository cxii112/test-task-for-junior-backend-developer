package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	taskdomain "example.com/taskservice/internal/domain/task"
	taskusecase "example.com/taskservice/internal/usecase/task"
)

type TaskHandler struct {
	usecase taskusecase.Usecase
}

func NewTaskHandler(usecase taskusecase.Usecase) *TaskHandler {
	return &TaskHandler{usecase: usecase}
}

func (h *TaskHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req taskMutationDTO
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	genReq := generationInputFromDTO(req.ChainGeneration)
	created, err := h.usecase.Create(r.Context(), taskusecase.CreateInput{
		Title:       req.Title,
		Description: req.Description,
		Status:      req.Status,
		StartAt:     req.StartAt,
		Deadline:    req.Deadline,
		Generation:  genReq,
	})
	if err != nil {
		writeUsecaseError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, newTaskDTO(created))
}

func (h *TaskHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	task, err := h.usecase.GetByID(r.Context(), id)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, newTaskDTO(task))
}

func (h *TaskHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var req taskMutationDTO
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	updated, err := h.usecase.Update(r.Context(), id, taskusecase.UpdateInput{
		Title:          req.Title,
		Description:    req.Description,
		Status:         req.Status,
		StartAt:        req.StartAt,
		Deadline:       req.Deadline,
		MakeStandalone: req.MakeStandalone,
	})
	if err != nil {
		writeUsecaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, newTaskDTO(updated))
}

func (h *TaskHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := h.usecase.Delete(r.Context(), id); err != nil {
		writeUsecaseError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TaskHandler) List(w http.ResponseWriter, r *http.Request) {
	tasks, err := h.usecase.List(r.Context())
	if err != nil {
		writeUsecaseError(w, err)
		return
	}

	response := make([]taskDTO, 0, len(tasks))
	for i := range tasks {
		response = append(response, newTaskDTO(&tasks[i]))
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *TaskHandler) GetChainByMasterID(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	chain, err := h.usecase.GetChainByMasterID(r.Context(), id)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}
	tasks := []taskDTO{}
	for _, task := range chain {
		dto := newTaskDTO(&task)
		tasks = append(tasks, dto)
	}
	writeJSON(w, http.StatusCreated, tasks)
}

func (h *TaskHandler) CreateChainFromTask(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req chainGenerationMutationDTO
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	genReq := generationInputFromDTO(&req)
	chain, err := h.usecase.CreateChainFromTask(r.Context(), id, *genReq)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}
	tasks := []taskDTO{}
	for _, task := range chain {
		dto := newTaskDTO(&task)
		tasks = append(tasks, dto)
	}
	writeJSON(w, http.StatusCreated, tasks)
}

func (h *TaskHandler) PropagateChain(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req chainPropagationMutationDTO
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	propReq := propagationInputFromDTO(req)
	chain, err := h.usecase.PropagateChain(r.Context(), id, propReq)
	if err != nil {
		writeUsecaseError(w, err)
		return
	}
	tasks := []taskDTO{}
	for _, task := range chain {
		dto := newTaskDTO(&task)
		tasks = append(tasks, dto)
	}
	writeJSON(w, http.StatusCreated, tasks)
}

func getIDFromRequest(r *http.Request) (int64, error) {
	rawID := mux.Vars(r)["id"]
	if rawID == "" {
		return 0, errors.New("missing task id")
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return 0, errors.New("invalid task id")
	}

	if id <= 0 {
		return 0, errors.New("invalid task id")
	}

	return id, nil
}

func decodeJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return err
	}

	return nil
}

func writeUsecaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, taskdomain.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, taskusecase.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{
		"error": err.Error(),
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(payload)
}

func generationInputFromDTO(i *chainGenerationMutationDTO) *taskusecase.GenerationInput {
	if i == nil {
		return nil
	}
	genReq := taskusecase.GenerationInput{}
	if i.Start != nil {
		genReq.Start = i.Start
	}
	if i.End != nil {
		genReq.End = i.End
	}
	scheduleReq := taskusecase.ScheduleInput{}
	switch {
	case i.Schedule.EveryEvenDay != nil:
		scheduleReq.EveryEvenDay = i.Schedule.EveryEvenDay
	case i.Schedule.EveryNthDay != nil:
		scheduleReq.EveryNthDay = i.Schedule.EveryNthDay
	case i.Schedule.EveryNthMonthDay != nil:
		scheduleReq.EveryNthMonthDay = i.Schedule.EveryNthMonthDay
	case len(i.Schedule.SparseDates) > 0:
		scheduleReq.SparseDates = i.Schedule.SparseDates
	}
	genReq.Schedule = scheduleReq
	return &genReq
}

func propagationInputFromDTO(i chainPropagationMutationDTO) taskusecase.PropagationInput {
	propReq := taskusecase.PropagationInput{}
	if i.Start != nil {
		propReq.Start = i.Start
	}
	if i.End != nil {
		propReq.End = i.End
	}
	return propReq
}
