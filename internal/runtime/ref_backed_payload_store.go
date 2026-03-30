package runtime

import (
	"context"
	"fmt"
	"strings"

	artifactblob "github.com/kobelakers/personal-cfo-os/internal/artifacts"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
)

type refBackedCheckpointStore struct {
	inner CheckpointStore
	blobs artifactblob.ArtifactBlobStore
}

func newRefBackedCheckpointStore(inner CheckpointStore, blobs artifactblob.ArtifactBlobStore) CheckpointStore {
	if inner == nil || blobs == nil {
		return inner
	}
	return &refBackedCheckpointStore{inner: inner, blobs: blobs}
}

func (s *refBackedCheckpointStore) Save(checkpoint CheckpointRecord) error {
	return s.inner.Save(checkpoint)
}

func (s *refBackedCheckpointStore) Load(workflowID string, checkpointID string) (CheckpointRecord, error) {
	return s.inner.Load(workflowID, checkpointID)
}

func (s *refBackedCheckpointStore) SaveResumeToken(token ResumeToken) error {
	return s.inner.SaveResumeToken(token)
}

func (s *refBackedCheckpointStore) LoadResumeToken(token string) (ResumeToken, error) {
	return s.inner.LoadResumeToken(token)
}

func (s *refBackedCheckpointStore) SavePayload(checkpointID string, payload CheckpointPayloadEnvelope) error {
	if !shouldRefBackCheckpointPayload(payload.Kind) {
		return s.inner.SavePayload(checkpointID, payload)
	}
	raw, err := marshalJSON(payload)
	if err != nil {
		return err
	}
	writeResult, err := s.blobs.WriteBlob(context.Background(), fmt.Sprintf("checkpoints/%s/payload.json", checkpointID), "application/json", []byte(raw))
	if err != nil {
		return err
	}
	return s.inner.SavePayload(checkpointID, CheckpointPayloadEnvelope{
		Kind:       payload.Kind,
		PayloadRef: writeResult.Ref.Location,
	})
}

func (s *refBackedCheckpointStore) LoadPayload(checkpointID string) (CheckpointPayloadEnvelope, error) {
	payload, err := s.inner.LoadPayload(checkpointID)
	if err != nil {
		return CheckpointPayloadEnvelope{}, err
	}
	if strings.TrimSpace(payload.PayloadRef) == "" {
		return payload, nil
	}
	ref, err := artifactblob.BlobRefFromLocation(payload.PayloadRef)
	if err != nil {
		return CheckpointPayloadEnvelope{}, err
	}
	readResult, err := s.blobs.ReadBlob(context.Background(), ref)
	if err != nil {
		return CheckpointPayloadEnvelope{}, err
	}
	var hydrated CheckpointPayloadEnvelope
	if err := unmarshalJSON(string(readResult.Content), &hydrated); err != nil {
		return CheckpointPayloadEnvelope{}, err
	}
	return hydrated, nil
}

type refBackedArtifactMetadataStore struct {
	inner ArtifactMetadataStore
	blobs artifactblob.ArtifactBlobStore
}

func newRefBackedArtifactMetadataStore(inner ArtifactMetadataStore, blobs artifactblob.ArtifactBlobStore) ArtifactMetadataStore {
	if inner == nil || blobs == nil {
		return inner
	}
	return &refBackedArtifactMetadataStore{inner: inner, blobs: blobs}
}

func (s *refBackedArtifactMetadataStore) SaveArtifact(workflowID string, taskID string, artifact reporting.WorkflowArtifact) error {
	if shouldRefBackArtifact(artifact.Kind) && strings.TrimSpace(artifact.ContentJSON) != "" {
		writeResult, err := s.blobs.WriteBlob(
			context.Background(),
			fmt.Sprintf("artifacts/%s/%s/%s.json", workflowID, taskID, artifact.ID),
			"application/json",
			[]byte(artifact.ContentJSON),
		)
		if err != nil {
			return err
		}
		artifact.Ref.Location = writeResult.Ref.Location
		artifact.ContentJSON = ""
	}
	return s.inner.SaveArtifact(workflowID, taskID, artifact)
}

func (s *refBackedArtifactMetadataStore) ListArtifactsByTask(taskID string) ([]reporting.WorkflowArtifact, error) {
	return s.inner.ListArtifactsByTask(taskID)
}

func (s *refBackedArtifactMetadataStore) ListArtifactsByWorkflow(workflowID string) ([]reporting.WorkflowArtifact, error) {
	return s.inner.ListArtifactsByWorkflow(workflowID)
}

func (s *refBackedArtifactMetadataStore) LoadArtifact(artifactID string) (reporting.WorkflowArtifact, bool, error) {
	artifact, ok, err := s.inner.LoadArtifact(artifactID)
	if err != nil || !ok {
		return artifact, ok, err
	}
	if !shouldRefBackArtifact(artifact.Kind) || strings.TrimSpace(artifact.Ref.Location) == "" || strings.TrimSpace(artifact.ContentJSON) != "" {
		return artifact, true, nil
	}
	ref, err := artifactblob.BlobRefFromLocation(artifact.Ref.Location)
	if err != nil {
		return reporting.WorkflowArtifact{}, false, err
	}
	readResult, err := s.blobs.ReadBlob(context.Background(), ref)
	if err != nil {
		return reporting.WorkflowArtifact{}, false, err
	}
	artifact.ContentJSON = string(readResult.Content)
	return artifact, true, nil
}

func shouldRefBackCheckpointPayload(kind CheckpointPayloadKind) bool {
	switch kind {
	case CheckpointPayloadKindFollowUpFinalizeResume:
		return true
	default:
		return false
	}
}

func shouldRefBackArtifact(kind reporting.ArtifactKind) bool {
	switch kind {
	case reporting.ArtifactKindMonthlyReviewReport,
		reporting.ArtifactKindDebtDecisionReport,
		reporting.ArtifactKindLifeEventAssessment,
		reporting.ArtifactKindTaxOptimizationReport,
		reporting.ArtifactKindPortfolioRebalanceReport,
		reporting.ArtifactKindBehaviorInterventionReport,
		reporting.ArtifactKindReplayBundle:
		return true
	default:
		return false
	}
}
