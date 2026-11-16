package notify

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

// apiError gives ErrChannelNotFound its HTTP form.
func apiError(err error) error {
	if errors.Is(err, ErrChannelNotFound) {
		return httpx.NotFound(ErrChannelNotFound.Error())
	}
	return err
}

func pathChannelID(r *http.Request) (int64, error) {
	id, ok := httpx.PathID(r, "id")
	if !ok {
		return 0, ErrChannelNotFound
	}
	return id, nil
}

func (s *Service) listChannels(w http.ResponseWriter, r *http.Request) error {
	channels, err := s.ListChannels(r.Context())
	if err != nil {
		return err
	}
	httpx.List(w, http.StatusOK, channels)
	return nil
}

type createChannelRequest struct {
	Name    string          `json:"name"`
	Kind    string          `json:"kind"`
	Config  json.RawMessage `json:"config"`
	Events  []string        `json:"events"`
	Enabled *bool           `json:"enabled"`
}

func (s *Service) createChannel(w http.ResponseWriter, r *http.Request) error {
	var req createChannelRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	channel, err := s.CreateChannel(r.Context(), CreateChannelInput(req))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusCreated, channel)
	return nil
}

// patchChannelRequest keeps every field nullable so an explicit null can be told from an absent field.
type patchChannelRequest struct {
	Name    httpx.Nullable[string]          `json:"name"`
	Config  httpx.Nullable[json.RawMessage] `json:"config"`
	Events  httpx.Nullable[[]string]        `json:"events"`
	Enabled httpx.Nullable[bool]            `json:"enabled"`
}

// input turns the request into an update, reporting a null for a field that cannot be cleared.
func (req patchChannelRequest) input() (UpdateChannelInput, error) {
	fields := httpx.FieldErrors{Message: channelSummary}
	if req.Name.Set && req.Name.Value == nil {
		fields.Add("name", httpx.CodeRequired)
	}
	if req.Config.Set && req.Config.Value == nil {
		fields.Add("config", httpx.CodeRequired)
	}
	if req.Events.Set && req.Events.Value == nil {
		fields.Add("events", httpx.CodeRequired)
	}
	if req.Enabled.Set && req.Enabled.Value == nil {
		fields.Add("enabled", httpx.CodeRequired)
	}
	in := UpdateChannelInput{Name: req.Name.Value, Events: req.Events.Value, Enabled: req.Enabled.Value}
	if req.Config.Value != nil {
		in.Config = *req.Config.Value
	}
	return in, fields.Err()
}

func (s *Service) patchChannel(w http.ResponseWriter, r *http.Request) error {
	id, err := pathChannelID(r)
	if err != nil {
		return err
	}
	var req patchChannelRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	in, err := req.input()
	if err != nil {
		return err
	}
	channel, err := s.UpdateChannel(r.Context(), id, in)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, channel)
	return nil
}

func (s *Service) deleteChannel(w http.ResponseWriter, r *http.Request) error {
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	id, err := pathChannelID(r)
	if err != nil {
		return err
	}
	if err := s.DeleteChannel(r.Context(), id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Service) testChannel(w http.ResponseWriter, r *http.Request) error {
	if err := httpx.RequireJSONOrNoBody(r); err != nil {
		return err
	}
	id, err := pathChannelID(r)
	if err != nil {
		return err
	}
	result, err := s.TestChannel(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, result)
	return nil
}

// currentUserID is the signed-in user, or an unauthorized error when the route is reached without one.
func currentUserID(r *http.Request) (int64, error) {
	u, ok := httpx.CurrentUser(r.Context())
	if !ok {
		return 0, httpx.Unauthorized("sign in required")
	}
	return u.ID, nil
}

// pageRequest reads the limit and cursor query parameters; the cursor is the id of the last item of the previous page.
func pageRequest(r *http.Request) (limit int, beforeID int64, err error) {
	limit = defaultPageSize
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, convErr := strconv.Atoi(raw)
		switch {
		case convErr != nil:
			return 0, 0, httpx.Reject("invalid query", "limit", httpx.CodeInvalid)
		case n < 1 || n > maxPageSize:
			return 0, 0, httpx.Reject("invalid query", "limit", httpx.CodeOutOfRange)
		}
		limit = n
	}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		id, convErr := strconv.ParseInt(raw, 10, 64)
		if convErr != nil || id < 1 {
			return 0, 0, httpx.Reject("invalid query", "cursor", httpx.CodeInvalid)
		}
		beforeID = id
	}
	return limit, beforeID, nil
}

func (s *Service) listInbox(w http.ResponseWriter, r *http.Request) error {
	userID, err := currentUserID(r)
	if err != nil {
		return err
	}
	unread, _, err := httpx.QueryBool(r, "unread")
	if err != nil {
		return err
	}
	limit, beforeID, err := pageRequest(r)
	if err != nil {
		return err
	}
	page, err := s.Inbox(r.Context(), userID, unread, beforeID, limit)
	if err != nil {
		return err
	}
	httpx.ListPage(w, http.StatusOK, page)
	return nil
}

func (s *Service) unreadCount(w http.ResponseWriter, r *http.Request) error {
	userID, err := currentUserID(r)
	if err != nil {
		return err
	}
	n, err := s.UnreadCount(r.Context(), userID)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int64{"count": n})
	return nil
}

type markReadRequest struct {
	IDs []int64 `json:"ids"`
	All bool    `json:"all"`
}

func (s *Service) markRead(w http.ResponseWriter, r *http.Request) error {
	userID, err := currentUserID(r)
	if err != nil {
		return err
	}
	var req markReadRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	fields := httpx.FieldErrors{Message: "invalid read request"}
	switch {
	case req.All && len(req.IDs) > 0:
		fields.Add("ids", httpx.CodeInvalid)
	case !req.All && len(req.IDs) == 0:
		fields.Add("ids", httpx.CodeRequired)
	case len(req.IDs) > maxReadIDs:
		fields.Add("ids", httpx.CodeTooLong)
	}
	for _, id := range req.IDs {
		if id < 1 {
			fields.Add("ids", httpx.CodeInvalid)
		}
	}
	if err := fields.Err(); err != nil {
		return err
	}
	if req.All {
		err = s.MarkAllRead(r.Context(), userID)
	} else {
		err = s.MarkRead(r.Context(), userID, req.IDs)
	}
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
