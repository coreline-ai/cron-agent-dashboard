package store

import (
	"errors"
	"testing"
)

func TestNormalizeErrClassifiesSQLiteConstraints(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{name: "unique", err: errors.New("UNIQUE constraint failed: agent.workspace_id, agent.name"), want: ErrConflict},
		{name: "check", err: errors.New("CHECK constraint failed: status IN ('open','done')"), want: ErrValidation},
		{name: "foreign key", err: errors.New("FOREIGN KEY constraint failed"), want: ErrValidation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeErr(tc.err)
			if !errors.Is(got, tc.want) {
				t.Fatalf("normalizeErr(%v)=%v, want errors.Is(..., %v)", tc.err, got, tc.want)
			}
		})
	}
}
