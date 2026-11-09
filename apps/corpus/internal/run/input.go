package run

// TaskQueue is the Temporal queue for corpus workflows.
const TaskQueue = "corpus"

// WorkflowName is the registered release workflow name.
const WorkflowName = "BuildRelease"

// WorkflowID returns the stable Temporal workflow ID for a release.
func WorkflowID(cc, rel string) string { return "build-" + cc + "-" + rel }

// BuildInput carries the release workflow's durable inputs.
type BuildInput struct {
	RunID        int64
	Country, Rel string
	Generation   int
	Stage, Until string
	Begin        bool
	HistoryLimit int
}
