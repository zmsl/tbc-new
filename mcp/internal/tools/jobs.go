package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/engine/jobs"
	"github.com/wowsims/tbc/mcp/internal/spec"
)

// Job ids are content hashes of the request, which is what keeps these tools stateless: an id
// means the same thing to any instance, survives a restart, and re-submitting the same run joins
// it rather than starting a second one.

type jobStatusInput struct {
	ID string `json:"id" jsonschema:"the job id returned by sim_run with async set"`
}

type jobStatusOutput struct {
	Job             jobs.Record `json:"job" jsonschema:"the job's current state"`
	PercentComplete float64     `json:"percentComplete" jsonschema:"progress through the requested iterations"`
}

func jobStatus(store jobs.Store) spec.Entry {
	return spec.Tool[jobStatusInput, jobStatusOutput]{
		Name:    "job_status",
		Title:   "Check a running simulation",
		Summary: "Reports how far a background simulation has got.",
		Details: "Poll this after starting a run with sim_run and async set. A job reported as 'stale' was\n" +
			"orphaned by a server restart; re-submitting the identical sim_run call starts it again and\n" +
			"produces the same answer, because runs are seeded and deterministic.",
		Examples: []spec.Example{
			{Description: "check progress", Args: `{"id": "3f2a1c9d4e5b6a7f"}`},
		},
		ReadOnly: true,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest, input jobStatusInput) (*mcp.CallToolResult, jobStatusOutput, error) {
			var output jobStatusOutput

			record, err := store.Status(input.ID)
			if err != nil {
				return nil, output, err
			}
			output.Job = record
			output.PercentComplete = round(record.PercentComplete())
			return nil, output, nil
		},
	}
}

type jobResultInput struct {
	ID         string `json:"id" jsonschema:"the job id"`
	SpellLimit int    `json:"spellLimit,omitempty" jsonschema:"how many spells to include in the damage breakdown. Defaults to 12."`
}

type jobResultOutput struct {
	Job    jobs.Record          `json:"job" jsonschema:"the job that produced this"`
	Result engine.ResultSummary `json:"result" jsonschema:"the simulation result"`
}

func jobResult(store jobs.Store) spec.Entry {
	return spec.Tool[jobResultInput, jobResultOutput]{
		Name:    "job_result",
		Title:   "Collect a finished simulation",
		Summary: "Returns the result of a completed background simulation.",
		Details: "Results stay readable until they are pruned, so asking twice is fine -- collecting a result\n" +
			"does not consume it.",
		Examples: []spec.Example{
			{Description: "collect a result", Args: `{"id": "3f2a1c9d4e5b6a7f"}`},
		},
		ReadOnly: true,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest, input jobResultInput) (*mcp.CallToolResult, jobResultOutput, error) {
			var output jobResultOutput

			record, err := store.Status(input.ID)
			if err != nil {
				return nil, output, err
			}
			if record.State != jobs.StateDone {
				return nil, output, fmt.Errorf("job %s is %s, not done%s", input.ID, record.State, describeJobError(record))
			}

			result, err := store.Result(input.ID)
			if err != nil {
				return nil, output, err
			}
			simRequest, err := store.Request(input.ID)
			if err != nil {
				return nil, output, err
			}

			output.Job = record
			output.Result = engine.SummarizeResult(simRequest, result, input.SpellLimit)
			return nil, output, nil
		},
	}
}

func describeJobError(record jobs.Record) string {
	if record.Error == "" {
		return ""
	}
	return ": " + record.Error
}

type jobCancelInput struct {
	ID string `json:"id" jsonschema:"the job id"`
}

type jobCancelOutput struct {
	Job jobs.Record `json:"job" jsonschema:"the job's state after the cancel"`
}

func jobCancel(store jobs.Store) spec.Entry {
	return spec.Tool[jobCancelInput, jobCancelOutput]{
		Name:    "job_cancel",
		Title:   "Stop a running simulation",
		Summary: "Cancels a background simulation.",
		Details: "Cancelling a job that has already finished does nothing and is not an error. A cancelled job\n" +
			"has no result; re-submit the same sim_run call to run it again.",
		Examples: []spec.Example{
			{Description: "stop a long run", Args: `{"id": "3f2a1c9d4e5b6a7f"}`},
		},
		// The one tool here that changes something. Cancelling twice is the same as cancelling
		// once, and cancelling a finished job does nothing.
		Idempotent: true,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest, input jobCancelInput) (*mcp.CallToolResult, jobCancelOutput, error) {
			var output jobCancelOutput

			record, err := store.Cancel(input.ID)
			if err != nil {
				return nil, output, err
			}
			output.Job = record
			return nil, output, nil
		},
	}
}

type jobListInput struct{}

type jobListOutput struct {
	Jobs []jobs.Record `json:"jobs" jsonschema:"known jobs, most recently started first"`
}

func jobList(store jobs.Store) spec.Entry {
	return spec.Tool[jobListInput, jobListOutput]{
		Name:    "job_list",
		Title:   "List simulations",
		Summary: "Lists background simulations and their states.",
		Details: "Includes jobs started by other sessions sharing the same job directory, and jobs that outlived\n" +
			"the server that started them.",
		Examples: []spec.Example{
			{Description: "what is running", Args: "{}"},
		},
		ReadOnly: true,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest, input jobListInput) (*mcp.CallToolResult, jobListOutput, error) {
			var output jobListOutput

			records, err := store.List()
			if err != nil {
				return nil, output, err
			}
			output.Jobs = records
			return nil, output, nil
		},
	}
}
