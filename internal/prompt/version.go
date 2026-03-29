package prompt

type RenderTraceRecorder interface {
	RecordPromptRender(trace PromptRenderTrace)
}
