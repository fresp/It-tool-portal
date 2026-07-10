package models

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

var ErrInvalidTool = errors.New("invalid tool")

type Tool struct {
	ID             string    `bson:"_id,omitempty" json:"id"`
	Name           string    `bson:"name" json:"name"`
	BaseURL        string    `bson:"base_url" json:"base_url"`
	IconURL        string    `bson:"icon_url" json:"icon_url"`
	AllowedGroups  []string  `bson:"allowed_groups" json:"allowed_groups"`
	HealthCheckURL *string   `bson:"health_check_url,omitempty" json:"health_check_url,omitempty"`
	IsActive       bool      `bson:"is_active" json:"is_active"`
	CreatedAt      time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time `bson:"updated_at" json:"updated_at"`
}

type CreateToolRequest struct {
	Name           string   `json:"name"`
	BaseURL        string   `json:"base_url"`
	IconURL        string   `json:"icon_url"`
	AllowedGroups  []string `json:"allowed_groups"`
	HealthCheckURL *string  `json:"health_check_url"`
	IsActive       *bool    `json:"is_active"`
}

type UpdateToolRequest struct {
	Name           *string  `json:"name"`
	BaseURL        *string  `json:"base_url"`
	IconURL        *string  `json:"icon_url"`
	AllowedGroups  []string `json:"allowed_groups"`
	HealthCheckURL *string  `json:"health_check_url"`
	IsActive       *bool    `json:"is_active"`
}

func (r *CreateToolRequest) Validate() error {
	r.Name = strings.TrimSpace(r.Name)
	r.BaseURL = strings.TrimSpace(r.BaseURL)
	r.IconURL = strings.TrimSpace(r.IconURL)
	r.AllowedGroups = normalizeGroups(r.AllowedGroups)
	normalizeOptionalString(r.HealthCheckURL)

	if r.Name == "" || !isHTTPURL(r.BaseURL) || !isHTTPURL(r.IconURL) || len(r.AllowedGroups) == 0 {
		return ErrInvalidTool
	}
	if r.HealthCheckURL != nil && !isHTTPURL(*r.HealthCheckURL) {
		return ErrInvalidTool
	}
	return nil
}

func (r *UpdateToolRequest) Validate() error {
	normalizeOptionalString(r.Name)
	normalizeOptionalString(r.BaseURL)
	normalizeOptionalString(r.IconURL)
	normalizeOptionalString(r.HealthCheckURL)
	if r.AllowedGroups != nil {
		r.AllowedGroups = normalizeGroups(r.AllowedGroups)
	}

	if r.Name != nil && *r.Name == "" {
		return ErrInvalidTool
	}
	if r.BaseURL != nil && !isHTTPURL(*r.BaseURL) {
		return ErrInvalidTool
	}
	if r.IconURL != nil && !isHTTPURL(*r.IconURL) {
		return ErrInvalidTool
	}
	if r.AllowedGroups != nil && len(r.AllowedGroups) == 0 {
		return ErrInvalidTool
	}
	if r.HealthCheckURL != nil && *r.HealthCheckURL != "" && !isHTTPURL(*r.HealthCheckURL) {
		return ErrInvalidTool
	}
	return nil
}

func normalizeOptionalString(value *string) {
	if value == nil {
		return
	}
	*value = strings.TrimSpace(*value)
}

func normalizeGroups(groups []string) []string {
	seen := make(map[string]struct{}, len(groups))
	normalized := make([]string, 0, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		normalized = append(normalized, group)
	}
	return normalized
}

func isHTTPURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}
