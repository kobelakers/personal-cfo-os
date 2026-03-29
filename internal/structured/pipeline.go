package structured

import (
	"context"
	"fmt"
)

import "github.com/kobelakers/personal-cfo-os/internal/model"

type Result[T any] struct {
	Value                 T
	Generation            model.StructuredGenerationResult
	FailureCategory       FailureCategory
	ValidationDiagnostics []string
	RepairAttempts        int
	FallbackUsed          bool
	FallbackReason        string
}

type Pipeline[T any] struct {
	Schema         Schema[T]
	Generator      model.StructuredGenerator
	RepairPolicy   RepairPolicy
	FallbackPolicy FallbackPolicy[T]
	TraceRecorder  TraceRecorder
}

func (p Pipeline[T]) Execute(ctx context.Context, request model.StructuredGenerationRequest) (Result[T], error) {
	repairPolicy := p.RepairPolicy
	if repairPolicy.MaxAttempts == 0 {
		repairPolicy = DefaultRepairPolicy()
	}
	initialRequest := request
	initialRequest.ModelRequest.GenerationPhase = model.GenerationPhaseInitial
	if initialRequest.ModelRequest.AttemptIndex == 0 {
		initialRequest.ModelRequest.AttemptIndex = 1
	}
	genResult, err := callGenerator(ctx, p.Generator, initialRequest)
	if err != nil {
		value, fallbackUsed, fallbackErr := p.executeFallback(FailureCategoryFallbackUsed, err.Error())
		if fallbackErr != nil {
			recordTrace(p.TraceRecorder, initialRequest, TraceRecord{
				SchemaName:      p.Schema.Name,
				ParseAttempts:   0,
				RepairAttempts:  0,
				FailureCategory: FailureCategoryFallbackFailed,
				FallbackUsed:    true,
				FallbackReason:  err.Error(),
			})
			return Result[T]{FailureCategory: FailureCategoryFallbackFailed, FallbackUsed: true, FallbackReason: err.Error()}, fallbackErr
		}
		recordTrace(p.TraceRecorder, initialRequest, TraceRecord{
			SchemaName:      p.Schema.Name,
			ParseAttempts:   0,
			RepairAttempts:  0,
			FailureCategory: FailureCategoryFallbackUsed,
			FallbackUsed:    fallbackUsed,
			FallbackReason:  err.Error(),
		})
		return Result[T]{Value: value, FailureCategory: FailureCategoryFallbackUsed, FallbackUsed: fallbackUsed, FallbackReason: err.Error()}, nil
	}

	parseAttempts := 1
	value, diagnostics, parseErr := p.parseAndValidate(genResult.Content)
	if parseErr == nil && len(diagnostics) == 0 {
		recordTrace(p.TraceRecorder, initialRequest, TraceRecord{
			SchemaName:     p.Schema.Name,
			ParseAttempts:  parseAttempts,
			RepairAttempts: 0,
		})
		return Result[T]{Value: value, Generation: genResult}, nil
	}

	lastCategory := FailureCategoryParseFailed
	if len(diagnostics) > 0 {
		lastCategory = FailureCategorySchemaInvalid
	}
	lastReason := parseErrString(parseErr, diagnostics)
	recordTrace(p.TraceRecorder, initialRequest, TraceRecord{
		SchemaName:            p.Schema.Name,
		ParseAttempts:         parseAttempts,
		RepairAttempts:        0,
		FailureCategory:       lastCategory,
		ValidationDiagnostics: diagnostics,
	})
	repairAttempts := 0
	lastRequest := initialRequest
	lastRaw := genResult.Content
	for repairAttempts < repairPolicy.MaxAttempts {
		repairAttempts++
		repairRequest := buildRepairRequest(initialRequest, p.Schema.Name, lastRaw, diagnostics, repairAttempts)
		lastRequest = repairRequest
		repaired, repairErr := callGenerator(ctx, p.Generator, repairRequest)
		if repairErr != nil {
			lastCategory = FailureCategoryRepairFailed
			lastReason = repairErr.Error()
			recordTrace(p.TraceRecorder, repairRequest, TraceRecord{
				SchemaName:            p.Schema.Name,
				ParseAttempts:         parseAttempts,
				RepairAttempts:        repairAttempts,
				FailureCategory:       FailureCategoryRepairFailed,
				ValidationDiagnostics: diagnostics,
			})
			break
		}
		parseAttempts++
		repairedValue, repairedDiagnostics, repairedParseErr := p.parseAndValidate(repaired.Content)
		if repairedParseErr == nil && len(repairedDiagnostics) == 0 {
			recordTrace(p.TraceRecorder, repairRequest, TraceRecord{
				SchemaName:     p.Schema.Name,
				ParseAttempts:  parseAttempts,
				RepairAttempts: repairAttempts,
			})
			return Result[T]{
				Value:                 repairedValue,
				Generation:            repaired,
				RepairAttempts:        repairAttempts,
				ValidationDiagnostics: nil,
			}, nil
		}
		if len(repairedDiagnostics) > 0 {
			lastCategory = FailureCategorySchemaInvalid
		} else {
			lastCategory = FailureCategoryParseFailed
		}
		lastReason = parseErrString(repairedParseErr, repairedDiagnostics)
		diagnostics = repairedDiagnostics
		lastRaw = repaired.Content
		recordTrace(p.TraceRecorder, repairRequest, TraceRecord{
			SchemaName:            p.Schema.Name,
			ParseAttempts:         parseAttempts,
			RepairAttempts:        repairAttempts,
			FailureCategory:       lastCategory,
			ValidationDiagnostics: diagnostics,
		})
	}

	value, fallbackUsed, fallbackErr := p.executeFallback(lastCategory, lastReason)
	if fallbackErr != nil {
		recordTrace(p.TraceRecorder, lastRequest, TraceRecord{
			SchemaName:            p.Schema.Name,
			ParseAttempts:         parseAttempts,
			RepairAttempts:        repairAttempts,
			FailureCategory:       FailureCategoryFallbackFailed,
			ValidationDiagnostics: diagnostics,
			FallbackUsed:          true,
			FallbackReason:        lastReason,
		})
		return Result[T]{
			FailureCategory:       FailureCategoryFallbackFailed,
			ValidationDiagnostics: diagnostics,
			RepairAttempts:        repairAttempts,
			FallbackUsed:          true,
			FallbackReason:        lastReason,
		}, fallbackErr
	}
	recordTrace(p.TraceRecorder, lastRequest, TraceRecord{
		SchemaName:            p.Schema.Name,
		ParseAttempts:         parseAttempts,
		RepairAttempts:        repairAttempts,
		FailureCategory:       lastCategory,
		ValidationDiagnostics: diagnostics,
		FallbackUsed:          fallbackUsed,
		FallbackReason:        lastReason,
	})
	return Result[T]{
		Value:                 value,
		FailureCategory:       FailureCategoryFallbackUsed,
		ValidationDiagnostics: diagnostics,
		RepairAttempts:        repairAttempts,
		FallbackUsed:          fallbackUsed,
		FallbackReason:        lastReason,
	}, nil
}

func (p Pipeline[T]) parseAndValidate(raw string) (T, []string, error) {
	var zero T
	if p.Schema.Parser == nil {
		return zero, nil, fmt.Errorf("schema %q parser is required", p.Schema.Name)
	}
	value, err := p.Schema.Parser.Parse(raw)
	if err != nil {
		return zero, nil, err
	}
	if p.Schema.Validator == nil {
		return value, nil, nil
	}
	diagnostics := p.Schema.Validator.Validate(value)
	return value, diagnostics, nil
}

func (p Pipeline[T]) executeFallback(category FailureCategory, reason string) (T, bool, error) {
	var zero T
	if p.FallbackPolicy.Execute == nil {
		return zero, false, fmt.Errorf("%s: %s", category, reason)
	}
	value, err := p.FallbackPolicy.Execute()
	if err != nil {
		return zero, true, err
	}
	return value, true, nil
}

func parseErrString(err error, diagnostics []string) string {
	if err != nil {
		return err.Error()
	}
	if len(diagnostics) == 0 {
		return ""
	}
	return fmt.Sprintf("%v", diagnostics)
}
