package maintenance

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/schedule"
	"github.com/reefshepherd/reefshepherd/internal/tanks"
)

// apiError gives the package's refusals, and the missing tank, their HTTP form.
func apiError(err error) error {
	for _, missing := range []error{tanks.ErrNotFound, errNoTask, errNoPreset, errNoCompletion} {
		if errors.Is(err, missing) {
			return httpx.NotFound(missing.Error())
		}
	}
	return err
}

// pathID reads the positive integer path value "id", failing with notFound when it is anything else.
func pathID(r *http.Request, notFound error) (int64, error) {
	id, ok := httpx.PathID(r, "id")
	if !ok {
		return 0, notFound
	}
	return id, nil
}

// clearable returns the value of a PATCH field that cannot be cleared, recording a required error in fields when it was sent as null.
func clearable[T any](fields *httpx.FieldErrors, name string, n httpx.Nullable[T]) *T {
	if n.Set && n.Value == nil {
		fields.Add(name, httpx.CodeRequired)
	}
	return n.Value
}

func (s *Service) getDefaults(w http.ResponseWriter, r *http.Request) error {
	d, err := s.Defaults(r.Context())
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, d)
	return nil
}

func (s *Service) listPresets(w http.ResponseWriter, r *http.Request) error {
	include, err := httpx.IncludeArchived(r)
	if err != nil {
		return err
	}
	presets, err := s.ListPresets(r.Context(), include)
	if err != nil {
		return err
	}
	httpx.List(w, http.StatusOK, presets)
	return nil
}

type createPresetRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Tankless    bool              `json:"tankless"`
	Schedule    *schedule.Columns `json:"schedule"`
}

func (s *Service) createPreset(w http.ResponseWriter, r *http.Request) error {
	var req createPresetRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	preset, err := s.CreatePreset(r.Context(), CreatePresetInput(req))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusCreated, preset)
	return nil
}

// patchPresetRequest keeps every field nullable so an explicit null can be told from an absent field.
type patchPresetRequest struct {
	Name        httpx.Nullable[string]           `json:"name"`
	Description httpx.Nullable[string]           `json:"description"`
	Tankless    httpx.Nullable[bool]             `json:"tankless"`
	Schedule    httpx.Nullable[schedule.Columns] `json:"schedule"`
	Archived    httpx.Nullable[bool]             `json:"archived"`
}

func (req patchPresetRequest) input() (UpdatePresetInput, error) {
	fields := httpx.FieldErrors{Message: "invalid preset"}
	in := UpdatePresetInput{
		Name:        clearable(&fields, "name", req.Name),
		Description: clearable(&fields, "description", req.Description),
		Tankless:    clearable(&fields, "tankless", req.Tankless),
		Schedule:    clearable(&fields, "schedule", req.Schedule),
		Archived:    req.Archived.Value,
	}
	if req.Archived.Set && req.Archived.Value == nil {
		fields.Add("archived", httpx.CodeInvalid)
	}
	return in, fields.Err()
}

func (s *Service) patchPreset(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, errNoPreset)
	if err != nil {
		return err
	}
	var req patchPresetRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	in, err := req.input()
	if err != nil {
		return err
	}
	preset, err := s.UpdatePreset(r.Context(), id, in)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, preset)
	return nil
}

func (s *Service) listTasks(w http.ResponseWriter, r *http.Request) error {
	var f TaskFilter
	tankID, set, err := httpx.QueryID(r, "tank_id")
	if err != nil {
		return err
	}
	if set {
		f.TankID = &tankID
	}
	if f.IncludeTankless, _, err = httpx.QueryBool(r, "include_tankless"); err != nil {
		return err
	}
	f.Status = r.URL.Query().Get("status")
	tasks, err := s.ListTasks(r.Context(), f)
	if err != nil {
		return err
	}
	httpx.List(w, http.StatusOK, tasks)
	return nil
}

func (s *Service) getTask(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, errNoTask)
	if err != nil {
		return err
	}
	task, err := s.GetTask(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, task)
	return nil
}

type createTaskRequest struct {
	TankID      *int64            `json:"tank_id"`
	PresetID    *int64            `json:"preset_id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Schedule    *schedule.Columns `json:"schedule"`
	Notify      *bool             `json:"notify"`
	Paused      *bool             `json:"paused"`
	LastDoneOn  *string           `json:"last_done_on"`
}

func (s *Service) createTask(w http.ResponseWriter, r *http.Request) error {
	var req createTaskRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	user, _ := httpx.CurrentUser(r.Context())
	task, err := s.CreateTask(r.Context(), user.ID, CreateTaskInput(req))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusCreated, task)
	return nil
}

// patchTaskRequest keeps every field nullable so an explicit null can be told from an absent field.
type patchTaskRequest struct {
	TankID      httpx.Nullable[int64]            `json:"tank_id"`
	Name        httpx.Nullable[string]           `json:"name"`
	Description httpx.Nullable[string]           `json:"description"`
	Schedule    httpx.Nullable[schedule.Columns] `json:"schedule"`
	Notify      httpx.Nullable[bool]             `json:"notify"`
	Paused      httpx.Nullable[bool]             `json:"paused"`
}

func (req patchTaskRequest) input() (UpdateTaskInput, error) {
	fields := httpx.FieldErrors{Message: "invalid task"}
	in := UpdateTaskInput{
		TankID:      req.TankID,
		Name:        clearable(&fields, "name", req.Name),
		Description: clearable(&fields, "description", req.Description),
		Schedule:    clearable(&fields, "schedule", req.Schedule),
		Notify:      clearable(&fields, "notify", req.Notify),
		Paused:      clearable(&fields, "paused", req.Paused),
	}
	return in, fields.Err()
}

func (s *Service) patchTask(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, errNoTask)
	if err != nil {
		return err
	}
	var req patchTaskRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	in, err := req.input()
	if err != nil {
		return err
	}
	task, err := s.UpdateTask(r.Context(), id, in)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, task)
	return nil
}

func (s *Service) deleteTask(w http.ResponseWriter, r *http.Request) error {
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	id, err := pathID(r, errNoTask)
	if err != nil {
		return err
	}
	if err := s.DeleteTask(r.Context(), id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type completeTaskRequest struct {
	DoneAt  *string `json:"done_at"`
	Outcome *string `json:"outcome"`
	Note    string  `json:"note"`
}

// completeTask records a completion; a request without a body records "done" now.
func (s *Service) completeTask(w http.ResponseWriter, r *http.Request) error {
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	id, err := pathID(r, errNoTask)
	if err != nil {
		return err
	}
	var req completeTaskRequest
	if r.Header.Get("Content-Type") != "" {
		if err := httpx.DecodeJSON(r, &req); err != nil {
			return err
		}
	}
	user, _ := httpx.CurrentUser(r.Context())
	result, err := s.Complete(r.Context(), id, user.ID, CompleteInput(req))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusCreated, result)
	return nil
}

func (s *Service) listCompletions(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, errNoTask)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	var limit int
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxPageSize {
			return httpx.Reject("invalid query", "limit", httpx.CodeOutOfRange)
		}
		limit = n
	}
	page, err := s.ListCompletions(r.Context(), id, q.Get("cursor"), limit)
	if err != nil {
		return err
	}
	httpx.ListPage(w, http.StatusOK, page)
	return nil
}

// patchCompletionRequest keeps every field nullable so an explicit null can be told from an absent field.
type patchCompletionRequest struct {
	DoneAt  httpx.Nullable[string] `json:"done_at"`
	Outcome httpx.Nullable[string] `json:"outcome"`
	Note    httpx.Nullable[string] `json:"note"`
}

func (req patchCompletionRequest) input() (UpdateCompletionInput, error) {
	fields := httpx.FieldErrors{Message: "invalid completion"}
	in := UpdateCompletionInput{
		DoneAt:  clearable(&fields, "done_at", req.DoneAt),
		Outcome: clearable(&fields, "outcome", req.Outcome),
		Note:    clearable(&fields, "note", req.Note),
	}
	return in, fields.Err()
}

func (s *Service) patchCompletion(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, errNoCompletion)
	if err != nil {
		return err
	}
	var req patchCompletionRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	in, err := req.input()
	if err != nil {
		return err
	}
	completion, err := s.UpdateCompletion(r.Context(), id, in)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, completion)
	return nil
}

func (s *Service) deleteCompletion(w http.ResponseWriter, r *http.Request) error {
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	id, err := pathID(r, errNoCompletion)
	if err != nil {
		return err
	}
	if err := s.DeleteCompletion(r.Context(), id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
