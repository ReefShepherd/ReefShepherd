package params

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/schedule"
	"github.com/reefshepherd/reefshepherd/internal/tanks"
)

// apiError gives the package's refusals, and the missing tank, their HTTP form.
func apiError(err error) error {
	for _, missing := range []error{tanks.ErrNotFound, errNoParameter, errNoResult} {
		if errors.Is(err, missing) {
			return httpx.NotFound(missing.Error())
		}
	}
	for _, conflict := range []error{errBuiltinParameter, errParameterHasData, errParameterArchived} {
		if errors.Is(err, conflict) {
			return httpx.Conflict(conflict.Error())
		}
	}
	return err
}

// pathID reads a positive integer path value, failing with notFound when it is not one.
func pathID(r *http.Request, name string, notFound error) (int64, error) {
	id, ok := httpx.PathID(r, name)
	if !ok {
		return 0, notFound
	}
	return id, nil
}

func (s *Service) listParameters(w http.ResponseWriter, r *http.Request) error {
	includeArchived, err := httpx.IncludeArchived(r)
	if err != nil {
		return err
	}
	defs, err := s.ListDefinitions(r.Context(), includeArchived)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"parameters": defs})
	return nil
}

type createParameterRequest struct {
	Name                    string   `json:"name"`
	UnitLabel               string   `json:"unit_label"`
	Decimals                *int     `json:"decimals"`
	DefaultLow              *float64 `json:"default_low"`
	DefaultHigh             *float64 `json:"default_high"`
	DefaultTestIntervalDays *int     `json:"default_test_interval_days"`
}

func (s *Service) createParameter(w http.ResponseWriter, r *http.Request) error {
	var req createParameterRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	def, err := s.CreateDefinition(r.Context(), CreateDefinitionInput(req))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusCreated, def)
	return nil
}

// patchParameterRequest keeps the clearable fields nullable so an explicit null can be told from an absent field.
type patchParameterRequest struct {
	Name                    *string                 `json:"name"`
	UnitLabel               *string                 `json:"unit_label"`
	Decimals                *int                    `json:"decimals"`
	DefaultLow              httpx.Nullable[float64] `json:"default_low"`
	DefaultHigh             httpx.Nullable[float64] `json:"default_high"`
	DefaultTestIntervalDays httpx.Nullable[int]     `json:"default_test_interval_days"`
}

func (s *Service) patchParameter(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id", errNoParameter)
	if err != nil {
		return err
	}
	var req patchParameterRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	def, err := s.UpdateDefinition(r.Context(), id, UpdateDefinitionInput(req))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, def)
	return nil
}

func (s *Service) archiveParameter(w http.ResponseWriter, r *http.Request) error {
	return s.setArchived(w, r, true)
}

func (s *Service) unarchiveParameter(w http.ResponseWriter, r *http.Request) error {
	return s.setArchived(w, r, false)
}

func (s *Service) setArchived(w http.ResponseWriter, r *http.Request, archived bool) error {
	id, err := pathID(r, "id", errNoParameter)
	if err != nil {
		return err
	}
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	def, err := s.SetDefinitionArchived(r.Context(), id, archived)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, def)
	return nil
}

func (s *Service) deleteParameter(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id", errNoParameter)
	if err != nil {
		return err
	}
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	if err := s.DeleteDefinition(r.Context(), id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Service) listTankParameters(w http.ResponseWriter, r *http.Request) error {
	tankID, err := pathID(r, "id", tanks.ErrNotFound)
	if err != nil {
		return err
	}
	out, err := s.ListTankParameters(r.Context(), tankID)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, out)
	return nil
}

// putTankParameterRequest keeps the clearable fields nullable so an explicit null can be told from an absent field.
type putTankParameterRequest struct {
	Enabled   *bool                   `json:"enabled"`
	IdealLow  httpx.Nullable[float64] `json:"ideal_low"`
	IdealHigh httpx.Nullable[float64] `json:"ideal_high"`
	Schedule  *schedule.Columns       `json:"schedule"`
	SortOrder *int                    `json:"sort_order"`
}

func (s *Service) putTankParameter(w http.ResponseWriter, r *http.Request) error {
	tankID, err := pathID(r, "id", tanks.ErrNotFound)
	if err != nil {
		return err
	}
	parameterID, err := pathID(r, "pid", errNoParameter)
	if err != nil {
		return err
	}
	var req putTankParameterRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	out, err := s.SetTankParameter(r.Context(), tankID, parameterID, SetTankParameterInput(req))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, out)
	return nil
}

type createResultsRequest struct {
	MeasuredAt *string `json:"measured_at"`
	Results    []struct {
		ParameterID int64    `json:"parameter_id"`
		Value       *float64 `json:"value"`
		Note        string   `json:"note"`
	} `json:"results"`
}

func (s *Service) createResults(w http.ResponseWriter, r *http.Request) error {
	tankID, err := pathID(r, "id", tanks.ErrNotFound)
	if err != nil {
		return err
	}
	user, _ := httpx.CurrentUser(r.Context())
	var req createResultsRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	in := CreateResultsInput{MeasuredAt: req.MeasuredAt}
	for _, e := range req.Results {
		in.Results = append(in.Results, ResultEntry(e))
	}
	created, err := s.CreateResults(r.Context(), tankID, user.ID, in)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"results": created})
	return nil
}

func (s *Service) listResults(w http.ResponseWriter, r *http.Request) error {
	tankID, err := pathID(r, "id", tanks.ErrNotFound)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	fields := httpx.FieldErrors{Message: "invalid query"}
	f := ResultFilter{Cursor: q.Get("cursor")}
	if raw := q.Get("parameter_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id < 1 {
			fields.Add("parameter_id", httpx.CodeInvalid)
		} else {
			f.ParameterID = &id
		}
	}
	f.From = queryTime(q.Get("from"), "from", &fields)
	f.To = queryTime(q.Get("to"), "to", &fields)
	if f.From != nil && f.To != nil && f.From.After(*f.To) {
		fields.Add("from", httpx.CodeOutOfRange)
	}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxPageSize {
			fields.Add("limit", httpx.CodeOutOfRange)
		} else {
			f.Limit = n
		}
	}
	if err := fields.Err(); err != nil {
		return err
	}
	page, err := s.ListResults(r.Context(), tankID, f)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, page)
	return nil
}

// queryTime reads an optional RFC 3339 query value.
func queryTime(raw, field string, fields *httpx.FieldErrors) *time.Time {
	if raw == "" {
		return nil
	}
	t, ok := parseTimestamp(raw)
	if !ok {
		fields.Add(field, httpx.CodeInvalid)
		return nil
	}
	return &t
}

func (s *Service) getSeries(w http.ResponseWriter, r *http.Request) error {
	tankID, err := pathID(r, "id", tanks.ErrNotFound)
	if err != nil {
		return err
	}
	parameterID, err := pathID(r, "pid", errNoParameter)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	fields := httpx.FieldErrors{Message: "invalid query"}
	from := queryTime(q.Get("from"), "from", &fields)
	to := queryTime(q.Get("to"), "to", &fields)
	if from != nil && to != nil && from.After(*to) {
		fields.Add("from", httpx.CodeOutOfRange)
	}
	if err := fields.Err(); err != nil {
		return err
	}
	out, err := s.GetSeries(r.Context(), tankID, parameterID, from, to)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, out)
	return nil
}

type patchResultRequest struct {
	MeasuredAt *string  `json:"measured_at"`
	Value      *float64 `json:"value"`
	Note       *string  `json:"note"`
}

func (s *Service) patchResult(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id", errNoResult)
	if err != nil {
		return err
	}
	var req patchResultRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	out, err := s.UpdateResult(r.Context(), id, UpdateResultInput(req))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, out)
	return nil
}

func (s *Service) deleteResult(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id", errNoResult)
	if err != nil {
		return err
	}
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	if err := s.DeleteResult(r.Context(), id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
