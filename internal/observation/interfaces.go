package observation

import "context"

type ObservationAdapter interface {
	SourceType() string
	Observe(ctx context.Context, request ObservationRequest) ([]EvidenceRecord, error)
}

type EvidenceExtractor interface {
	Extract(ctx context.Context, raw RawObservation) ([]EvidenceClaim, error)
}

type EvidenceNormalizer interface {
	Normalize(ctx context.Context, record EvidenceRecord) (EvidenceRecord, error)
}
