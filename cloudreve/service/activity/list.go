package activity

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/activity"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
)

const (
	defaultActivityPageSize = 20
	maxActivityPageSize     = 200
)

type (
	// Response is the public representation of an activity entry.
	Response struct {
		ID         int            `json:"id"`
		UserID     int            `json:"user_id,omitempty"`
		Action     string         `json:"action"`
		TargetType string         `json:"target_type"`
		Target     string         `json:"target,omitempty"`
		FileID     int            `json:"file_id,omitempty"`
		IP         string         `json:"ip,omitempty"`
		UserAgent  string         `json:"user_agent,omitempty"`
		Metadata   map[string]any `json:"metadata,omitempty"`
		CreatedAt  time.Time      `json:"created_at"`
	}

	// ListResponse is a paginated activity list.
	ListResponse struct {
		Pagination *inventory.PaginationResults `json:"pagination"`
		Activities []Response                   `json:"activities"`
	}

	// AdminListActivitiesService lists all activity entries with filters.
	AdminListActivitiesService struct {
		Page     int    `form:"page" binding:"omitempty,min=1"`
		PageSize int    `form:"page_size" binding:"omitempty,min=1,max=200"`
		Action   string `form:"action"`
		UserID   int    `form:"user_id" binding:"omitempty,min=1"`
		IP       string `form:"ip"`
		From     string `form:"from"`
		To       string `form:"to"`
	}
	AdminListActivitiesParamCtx struct{}

	// AdminActivityDetailService returns a single activity entry.
	AdminActivityDetailService struct {
		ID int `uri:"id" binding:"required,min=1"`
	}
	AdminActivityDetailParamCtx struct{}

	// UserActivityListService lists the authenticated user's own activity.
	UserActivityListService struct {
		Page     int `form:"page" binding:"omitempty,min=1"`
		PageSize int `form:"page_size" binding:"omitempty,min=1,max=200"`
		// Scope can be "all" (default), "file" or "login".
		Scope  string `form:"scope"`
		Action string `form:"action"`
	}
	UserActivityListParamCtx struct{}
)

// List returns the paginated, filtered activity list for administrators.
func (s *AdminListActivitiesService) List(c *gin.Context) (*ListResponse, error) {
	dep := dependency.FromContext(c)
	q := dep.DBClient().Activity.Query()

	if s.Action != "" {
		q = q.Where(activity.ActionEQ(s.Action))
	}
	if s.UserID > 0 {
		q = q.Where(activity.UserIDEQ(s.UserID))
	}
	if s.IP != "" {
		q = q.Where(activity.IPContains(s.IP))
	}
	if from, ok := parseActivityTime(s.From, false); ok {
		q = q.Where(activity.CreatedAtGTE(from))
	}
	if to, ok := parseActivityTime(s.To, true); ok {
		q = q.Where(activity.CreatedAtLTE(to))
	}

	return paginateActivities(c, q, s.Page, s.PageSize)
}

// Get returns a single activity entry for administrators.
func (s *AdminActivityDetailService) Get(c *gin.Context) (*Response, error) {
	dep := dependency.FromContext(c)
	entry, err := dep.DBClient().Activity.Get(c, s.ID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, serializer.NewError(serializer.CodeNotFound, "Activity not found", err)
		}
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get activity", err)
	}

	res := buildActivityResponse(entry)
	return &res, nil
}

// List returns the authenticated user's own paginated activity.
func (s *UserActivityListService) List(c *gin.Context) (*ListResponse, error) {
	dep := dependency.FromContext(c)
	uid := inventory.UserIDFromContext(c)
	if uid <= 0 {
		return nil, serializer.NewError(serializer.CodeCredentialInvalid, "Authentication required", nil)
	}

	q := dep.DBClient().Activity.Query().Where(activity.UserIDEQ(uid))
	switch s.Scope {
	case "file":
		q = q.Where(activity.TargetTypeIn(TargetTypeFile, TargetTypeShare))
	case "login":
		q = q.Where(activity.ActionIn(ActionLoginSuccess, ActionLoginFailed))
	}
	if s.Action != "" {
		q = q.Where(activity.ActionEQ(s.Action))
	}

	return paginateActivities(c, q, s.Page, s.PageSize)
}

func paginateActivities(c context.Context, q *ent.ActivityQuery, page, pageSize int) (*ListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultActivityPageSize
	}
	if pageSize > maxActivityPageSize {
		pageSize = maxActivityPageSize
	}

	total, err := q.Clone().Count(c)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to count activities", err)
	}

	rows, err := q.
		Order(ent.Desc(activity.FieldCreatedAt), ent.Desc(activity.FieldID)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(c)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list activities", err)
	}

	res := make([]Response, 0, len(rows))
	for _, row := range rows {
		res = append(res, buildActivityResponse(row))
	}

	return &ListResponse{
		Pagination: &inventory.PaginationResults{
			Page:       page,
			PageSize:   pageSize,
			TotalItems: total,
		},
		Activities: res,
	}, nil
}

func buildActivityResponse(entry *ent.Activity) Response {
	return Response{
		ID:         entry.ID,
		UserID:     entry.UserID,
		Action:     entry.Action,
		TargetType: entry.TargetType,
		Target:     entry.Target,
		FileID:     entry.FileID,
		IP:         entry.IP,
		UserAgent:  entry.UserAgent,
		Metadata:   entry.Metadata,
		CreatedAt:  entry.CreatedAt,
	}
}

// parseActivityTime accepts RFC3339 timestamps, Unix seconds or YYYY-MM-DD.
// Date-only upper bounds are rounded to the end of the day.
func parseActivityTime(value string, endOfDay bool) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}

	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, true
	}

	if unix, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.Unix(unix, 0), true
	}

	if t, err := time.ParseInLocation("2006-01-02", value, time.Local); err == nil {
		if endOfDay {
			return t.Add(24*time.Hour - time.Nanosecond), true
		}
		return t, true
	}

	return time.Time{}, false
}
