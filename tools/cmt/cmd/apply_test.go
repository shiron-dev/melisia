package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/syncer"
)

func TestVerifyExpectedPlanSHA256(t *testing.T) {
	t.Parallel()

	plan := &syncer.SyncPlan{
		HostPlans: []syncer.HostPlan{
			{
				Host: config.HostEntry{
					Name: "web",
					Host: "web.example.com",
					User: "deploy",
					Port: 22,
				},
				Projects: []syncer.ProjectPlan{
					{
						ProjectName: "app",
						RemoteDir:   "/srv/app",
						Files: []syncer.FilePlan{
							{
								RelativePath: "compose.yml",
								RemotePath:   "/srv/app/compose.yml",
								Action:       syncer.ActionAdd,
							},
						},
					},
				},
			},
		},
	}
	digest := syncer.PlanDigestSHA256(plan)

	tests := []struct {
		name           string
		expectedDigest string
		wantMismatch   bool
	}{
		{
			name:           "empty expectation allows apply",
			expectedDigest: "",
		},
		{
			name:           "matching digest allows apply",
			expectedDigest: digest,
		},
		{
			name:           "matching digest is case insensitive",
			expectedDigest: strings.ToUpper(digest),
		},
		{
			name:           "mismatched digest blocks apply",
			expectedDigest: strings.Repeat("0", len(digest)),
			wantMismatch:   true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := verifyExpectedPlanSHA256(plan, testCase.expectedDigest)
			if testCase.wantMismatch {
				if !errors.Is(err, errPlanDigestMismatch) {
					t.Fatalf("expected errPlanDigestMismatch, got %v", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("verifyExpectedPlanSHA256() returned error: %v", err)
			}
		})
	}
}
