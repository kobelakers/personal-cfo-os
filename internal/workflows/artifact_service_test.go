package workflows

import (
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/tools"
)

func TestArtifactServiceProducesStructuredArtifact(t *testing.T) {
	service := ArtifactService{
		Tool:     tools.GenerateTaskArtifactTool{},
		Producer: StaticArtifactProducer{Now: func() time.Time { return time.Date(2026, 3, 28, 18, 0, 0, 0, time.UTC) }},
		Now:      func() time.Time { return time.Date(2026, 3, 28, 18, 0, 0, 0, time.UTC) },
	}
	artifact, err := service.Produce("workflow-1", "task-1", ArtifactKindVerificationReport, map[string]any{"status": "pass"}, "verification summary", "verification_pipeline")
	if err != nil {
		t.Fatalf("produce artifact: %v", err)
	}
	if artifact.Kind != ArtifactKindVerificationReport || artifact.Ref.Summary == "" || artifact.ContentJSON == "" {
		t.Fatalf("expected artifact content and summary, got %+v", artifact)
	}
}
