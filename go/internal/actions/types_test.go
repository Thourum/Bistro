package actions

import (
	"testing"
	"time"
)

func TestActionRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     *ActionRequest
		wantErr bool
	}{
		{
			name: "valid approve action",
			req: &ActionRequest{
				PRID:   "github:owner/repo:42",
				Action: "approve",
				Body:   "Looks good!",
			},
			wantErr: false,
		},
		{
			name: "missing PRID",
			req: &ActionRequest{
				Action: "approve",
				Body:   "Looks good!",
			},
			wantErr: true,
		},
		{
			name: "invalid action type",
			req: &ActionRequest{
				PRID:   "github:owner/repo:42",
				Action: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestActionResultSuccess(t *testing.T) {
	result := &ActionResult{
		PRID:      "github:owner/repo:42",
		Action:    "approve",
		Success:   true,
		Timestamp: time.Now(),
	}

	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.Error != "" {
		t.Errorf("expected no error, got %s", result.Error)
	}
}

func TestActionResultFailure(t *testing.T) {
	result := &ActionResult{
		PRID:      "github:owner/repo:42",
		Action:    "approve",
		Success:   false,
		Error:     "rate limit exceeded",
		Timestamp: time.Now(),
	}

	if result.Success {
		t.Error("expected Success=false")
	}
	if result.Error != "rate limit exceeded" {
		t.Errorf("expected error message, got %s", result.Error)
	}
}

func TestResolveThreadCommandValidation(t *testing.T) {
	cases := []struct {
		name    string
		req     ActionRequest
		wantErr bool
	}{
		{
			name:    "valid resolve",
			req:     ActionRequest{PRID: "pr1", Action: ResolveThreadCommand, Body: "PRRT_abc"},
			wantErr: false,
		},
		{
			name:    "missing thread id (body)",
			req:     ActionRequest{PRID: "pr1", Action: ResolveThreadCommand, Body: ""},
			wantErr: true,
		},
		{
			name:    "missing pr id",
			req:     ActionRequest{PRID: "", Action: ResolveThreadCommand, Body: "PRRT_abc"},
			wantErr: true,
		},
		{
			name:    "valid unresolve",
			req:     ActionRequest{PRID: "pr1", Action: UnresolveThreadCommand, Body: "PRRT_abc"},
			wantErr: false,
		},
		{
			name:    "unresolve missing body",
			req:     ActionRequest{PRID: "pr1", Action: UnresolveThreadCommand, Body: ""},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
