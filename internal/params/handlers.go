package params

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

type handlerFunc func(w http.ResponseWriter, r *http.Request) error

func (s *Service) handle(h handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			httpx.Fail(s.Logger, w, r, err)
		}
	}
}

// pathID reads a positive integer path value, answering 404 with what was not found when it is not one.
func pathID(r *http.Request, name, what string) (int64, error) {
	id, ok := httpx.PathID(r, name)
	if !ok {
		return 0, notFound(what)
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

// patchParameterRequest keeps the clearable fields raw so an explicit null can be told from an absent field.
type patchParameterRequest struct {
	Name                    *string         `json:"name"`
	UnitLabel               *string         `json:"unit_label"`
	Decimals                *int            `json:"decimals"`
	DefaultLow              json.RawMessage `json:"default_low"`
	DefaultHigh             json.RawMessage `json:"default_high"`
	DefaultTestIntervalDays json.RawMessage `json:"default_test_interval_days"`
}

func (s *Service) patchParameter(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id", "parameter")
	if err != nil {
		return err
	}
	var req patchParameterRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	fields := fieldErrors{}
	in := UpdateDefinitionInput{
		Name: req.Name, UnitLabel: req.UnitLabel, Decimals: req.Decimals,
		DefaultLow:              nullableOf[float64](req.DefaultLow, "default_low", "a number", fields),
		DefaultHigh:             nullableOf[float64](req.DefaultHigh, "default_high", "a number", fields),
		DefaultTestIntervalDays: nullableOf[int](req.DefaultTestIntervalDays, "default_test_interval_days", "a whole number", fields),
	}
	if err := fields.err("invalid parameter"); err != nil {
		return err
	}
	def, err := s.UpdateDefinition(r.Context(), id, in)
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
	id, err := pathID(r, "id", "parameter")
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
	id, err := pathID(r, "id", "parameter")
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
	tankID, err := pathID(r, "id", "tank")
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

// putTankParameterRequest keeps the clearable fields raw so an explicit null can be told from an absent field.
type putTankParameterRequest struct {
	Enabled   *bool           `json:"enabled"`
	IdealLow  json.RawMessage `json:"ideal_low"`
	IdealHigh json.RawMessage `json:"ideal_high"`
	Schedule  *Schedule       `json:"schedule"`
	SortOrder *int            `json:"sort_order"`
}

func (s *Service) putTankParameter(w http.ResponseWriter, r *http.Request) error {
	tankID, err := pathID(r, "id", "tank")
	if err != nil {
		return err
	}
	parameterID, err := pathID(r, "pid", "parameter")
	if err != nil {
		return err
	}
	var req putTankParameterRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	fields := fieldErrors{}
	in := SetTankParameterInput{
		Enabled: req.Enabled, Schedule: req.Schedule, SortOrder: req.SortOrder,
		IdealLow:  nullableOf[float64](req.IdealLow, "ideal_low", "a number", fields),
		IdealHigh: nullableOf[float64](req.IdealHigh, "ideal_high", "a number", fields),
	}
	if err := fields.err("invalid parameter settings"); err != nil {
		return err
	}
	out, err := s.SetTankParameter(r.Context(), tankID, parameterID, in)
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
	tankID, err := pathID(r, "id", "tank")
	if err != nil {
		return err
	}
	user, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
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
	tankID, err := pathID(r, "id", "tank")
	if err != nil {
		return err
	}
	q := r.URL.Query()
	fields := fieldErrors{}
	f := ResultFilter{Cursor: q.Get("cursor")}
	if raw := q.Get("parameter_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id < 1 {
			fields.add("parameter_id", "must be a positive whole number")
		} else {
			f.ParameterID = &id
		}
	}
	f.From = queryTime(q.Get("from"), "from", fields)
	f.To = queryTime(q.Get("to"), "to", fields)
	if f.From != nil && f.To != nil && f.From.After(*f.To) {
		fields.add("from", "must not be after to")
	}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxPageSize {
			fields.add("limit", "must be a whole number from 1 to "+strconv.Itoa(maxPageSize))
		} else {
			f.Limit = n
		}
	}
	if err := fields.err("invalid query"); err != nil {
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
func queryTime(raw, field string, fields fieldErrors) *time.Time {
	if raw == "" {
		return nil
	}
	t, err := parseTimestamp(raw)
	if err != nil {
		fields.add(field, err.Error())
		return nil
	}
	return &t
}

func (s *Service) getSeries(w http.ResponseWriter, r *http.Request) error {
	tankID, err := pathID(r, "id", "tank")
	if err != nil {
		return err
	}
	parameterID, err := pathID(r, "pid", "parameter")
	if err != nil {
		return err
	}
	q := r.URL.Query()
	fields := fieldErrors{}
	from := queryTime(q.Get("from"), "from", fields)
	to := queryTime(q.Get("to"), "to", fields)
	if from != nil && to != nil && from.After(*to) {
		fields.add("from", "must not be after to")
	}
	if err := fields.err("invalid query"); err != nil {
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
	id, err := pathID(r, "id", "result")
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
	id, err := pathID(r, "id", "result")
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
