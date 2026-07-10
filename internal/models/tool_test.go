package models

import "testing"

func TestCreateToolRequestValidate_rejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		request CreateToolRequest
	}{
		{
			name: "missing name",
			request: CreateToolRequest{
				BaseURL:       "https://example.com",
				IconURL:       "https://example.com/icon.svg",
				AllowedGroups: []string{"dev"},
			},
		},
		{
			name: "invalid base url",
			request: CreateToolRequest{
				Name:          "Docs",
				BaseURL:       "not-a-url",
				IconURL:       "https://example.com/icon.svg",
				AllowedGroups: []string{"dev"},
			},
		},
		{
			name: "invalid icon url",
			request: CreateToolRequest{
				Name:          "Docs",
				BaseURL:       "https://example.com",
				IconURL:       "not-a-url",
				AllowedGroups: []string{"dev"},
			},
		},
		{
			name: "empty groups",
			request: CreateToolRequest{
				Name:          "Docs",
				BaseURL:       "https://example.com",
				IconURL:       "https://example.com/icon.svg",
				AllowedGroups: []string{},
			},
		},
		{
			name: "invalid health check url",
			request: CreateToolRequest{
				Name:           "Docs",
				BaseURL:        "https://example.com",
				IconURL:        "https://example.com/icon.svg",
				AllowedGroups:  []string{"dev"},
				HealthCheckURL: stringPtr("not-a-url"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.request.Validate(); err == nil {
				t.Fatalf("CreateToolRequest.Validate() error = nil, want validation error")
			}
		})
	}
}

func TestCreateToolRequestValidate_normalizesValidInput(t *testing.T) {
	request := CreateToolRequest{
		Name:           "  Docs  ",
		BaseURL:        " https://docs.example.com/app ",
		IconURL:        " https://docs.example.com/icon.svg ",
		AllowedGroups:  []string{" dev ", "ops", "dev"},
		HealthCheckURL: stringPtr(" https://docs.example.com/health "),
	}

	if err := request.Validate(); err != nil {
		t.Fatalf("CreateToolRequest.Validate() error = %v, want nil", err)
	}
	if request.Name != "Docs" {
		t.Fatalf("Name = %q, want Docs", request.Name)
	}
	if request.AllowedGroups[0] != "dev" || request.AllowedGroups[1] != "ops" {
		t.Fatalf("AllowedGroups = %#v, want normalized unique groups", request.AllowedGroups)
	}
	if request.HealthCheckURL == nil || *request.HealthCheckURL != "https://docs.example.com/health" {
		t.Fatalf("HealthCheckURL = %#v, want normalized health URL", request.HealthCheckURL)
	}
}

func TestUpdateToolRequestValidate_rejectsInvalidPatch(t *testing.T) {
	request := UpdateToolRequest{
		BaseURL: stringPtr("not-a-url"),
	}

	if err := request.Validate(); err == nil {
		t.Fatalf("UpdateToolRequest.Validate() error = nil, want validation error")
	}
}

func stringPtr(value string) *string {
	return &value
}
