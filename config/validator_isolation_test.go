package config_test

import (
	"context"
	"testing"

	"github.com/nostosx/servex/config"
)

func TestValidatorSnapshotAssignmentDoesNotAffectOtherValidators(t *testing.T) {
	c, err := config.New(t.Context(), []config.Source{src(map[string]any{"port": 8080})},
		config.WithValidator(func(_ context.Context, candidate *config.Snapshot) error {
			// Replacing the caller-owned view must not replace a later validator's view.
			*candidate = config.Snapshot{}
			return nil
		}),
		config.WithValidator(func(_ context.Context, candidate *config.Snapshot) error {
			if candidate.Int("port") != 8080 {
				t.Error("validator received a replaced snapshot")
			}
			return nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Int("port") != 8080 {
		t.Fatal("validator replaced the published tree")
	}
}
