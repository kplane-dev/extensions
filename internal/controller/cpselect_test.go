package controller_test

import (
	"testing"

	"github.com/kplane-dev/extensions/internal/controller"
)

func TestCPMatches(t *testing.T) {
	tests := []struct {
		name          string
		controlPlanes []string
		cpName        string
		want          bool
	}{
		{name: "wildcard matches any", controlPlanes: []string{"*"}, cpName: "test", want: true},
		{name: "wildcard matches another", controlPlanes: []string{"*"}, cpName: "production", want: true},
		{name: "exact match", controlPlanes: []string{"test", "staging"}, cpName: "test", want: true},
		{name: "exact match second", controlPlanes: []string{"test", "staging"}, cpName: "staging", want: true},
		{name: "no match", controlPlanes: []string{"test", "staging"}, cpName: "production", want: false},
		{name: "empty list no match", controlPlanes: []string{}, cpName: "test", want: false},
		{name: "wildcard not partial", controlPlanes: []string{"test*"}, cpName: "test-1", want: false},
		{name: "single exact match", controlPlanes: []string{"production"}, cpName: "production", want: true},
		{name: "single no match", controlPlanes: []string{"production"}, cpName: "test", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := controller.CPMatches(tt.controlPlanes, tt.cpName)
			if got != tt.want {
				t.Errorf("CPMatches(%v, %q) = %v, want %v", tt.controlPlanes, tt.cpName, got, tt.want)
			}
		})
	}
}
