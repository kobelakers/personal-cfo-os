package workflows

import (
	"encoding/json"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/tools"
)

type ArtifactService struct {
	Tool     tools.GenerateTaskArtifactTool
	Producer ArtifactProducer
	Now      func() time.Time
}

func (s ArtifactService) Produce(
	workflowID string,
	taskID string,
	kind ArtifactKind,
	content any,
	summary string,
	producedBy string,
) (WorkflowArtifact, error) {
	contentJSON, err := s.Tool.Generate(content)
	if err != nil {
		return WorkflowArtifact{}, err
	}
	producer := s.Producer
	if producer == nil {
		producer = StaticArtifactProducer{Now: s.Now}
	}
	return producer.ProduceArtifact(workflowID, taskID, kind, contentJSON, summary, producedBy), nil
}

func (s ArtifactService) ProduceJSON(
	workflowID string,
	taskID string,
	kind ArtifactKind,
	content any,
	summary string,
	producedBy string,
) (WorkflowArtifact, error) {
	payload, err := json.Marshal(content)
	if err != nil {
		return WorkflowArtifact{}, err
	}
	producer := s.Producer
	if producer == nil {
		producer = StaticArtifactProducer{Now: s.Now}
	}
	return producer.ProduceArtifact(workflowID, taskID, kind, string(payload), summary, producedBy), nil
}
