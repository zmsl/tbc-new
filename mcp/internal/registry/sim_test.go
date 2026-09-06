package registry_test

import (
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type metric struct {
	Avg    float64 `json:"avg"`
	Stdev  float64 `json:"stdev"`
	StdErr float64 `json:"stderr"`
}

type resultSummary struct {
	Dps             metric  `json:"dps"`
	Iterations      int32   `json:"iterations"`
	DurationSeconds float64 `json:"durationSeconds"`
	Spells          []struct {
		Name    string  `json:"name"`
		SpellID int32   `json:"spellId"`
		Casts   float64 `json:"casts"`
		Dps     float64 `json:"dps"`
	} `json:"spells"`
}

type jobRecord struct {
	ID                  string `json:"id"`
	State               string `json:"state"`
	Link                string `json:"link"`
	CompletedIterations int32  `json:"completedIterations"`
	TotalIterations     int32  `json:"totalIterations"`
	Error               string `json:"error"`
}

type simRunOutput struct {
	Result *resultSummary `json:"result"`
	Job    *jobRecord     `json:"job"`
	Link   string         `json:"link"`
}

func TestSimRun(t *testing.T) {
	session := connect(t)

	output := callTool[simRunOutput](t, session, "sim_run", map[string]any{
		"spec":       "SmitePriest",
		"gearSet":    "p3",
		"iterations": 200,
	})

	if output.Result == nil {
		t.Fatal("a synchronous run returned no result")
	}
	if output.Result.Dps.Avg <= 0 {
		t.Errorf("dps = %v", output.Result.Dps.Avg)
	}
	if output.Result.Dps.StdErr <= 0 {
		t.Error("no standard error reported; a result without one cannot be compared to another")
	}
	if output.Result.Iterations != 200 {
		t.Errorf("iterations = %d, want 200", output.Result.Iterations)
	}
	if output.Result.DurationSeconds != 180 {
		t.Errorf("duration = %v, want the default long fight", output.Result.DurationSeconds)
	}
	if output.Link == "" {
		t.Error("no share link returned")
	}

	// The breakdown is what explains the number, so it has to be readable rather than a list of
	// spell ids.
	if len(output.Result.Spells) == 0 {
		t.Fatal("no spell breakdown")
	}
	var foundSmite bool
	for _, spell := range output.Result.Spells {
		if spell.Name == "" {
			t.Errorf("spell %d has no name", spell.SpellID)
		}
		if spell.Name == "Smite" || spell.Name == "Smite (Rank 10)" {
			foundSmite = true
			if spell.Casts <= 0 {
				t.Error("smite was listed but never cast")
			}
		}
	}
	if !foundSmite {
		t.Errorf("a smite priest's breakdown does not mention Smite: %+v", output.Result.Spells)
	}
}

// Determinism is what makes results comparable and job ids meaningful.
func TestSimRunIsDeterministic(t *testing.T) {
	session := connect(t)

	arguments := map[string]any{"spec": "SmitePriest", "gearSet": "p3", "iterations": 100}
	first := callTool[simRunOutput](t, session, "sim_run", arguments)
	second := callTool[simRunOutput](t, session, "sim_run", arguments)

	if first.Result.Dps.Avg != second.Result.Dps.Avg {
		t.Errorf("same arguments gave %v then %v", first.Result.Dps.Avg, second.Result.Dps.Avg)
	}

	// A different seed should move the number a little, or the seed is not being used.
	withSeed := callTool[simRunOutput](t, session, "sim_run", map[string]any{
		"spec": "SmitePriest", "gearSet": "p3", "iterations": 100, "seed": 7,
	})
	if withSeed.Result.Dps.Avg == first.Result.Dps.Avg {
		t.Error("changing the seed changed nothing")
	}
}

func TestSimRunRejectsOversizedSyncRuns(t *testing.T) {
	session := connect(t)

	message := toolError(t, session, "sim_run", map[string]any{
		"spec":       "SmitePriest",
		"iterations": 100000,
	})
	if message == "" {
		t.Fatal("expected an error explaining the limit")
	}
}

func waitForJob(t *testing.T, session *mcp.ClientSession, id string) jobRecord {
	t.Helper()

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		status := callTool[struct {
			Job             jobRecord `json:"job"`
			PercentComplete float64   `json:"percentComplete"`
		}](t, session, "job_status", map[string]any{"id": id})

		if status.Job.State != "running" {
			return status.Job
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("job %s did not finish within a minute", id)
	return jobRecord{}
}

func TestAsyncSimJob(t *testing.T) {
	session := connect(t)

	arguments := map[string]any{
		"spec":       "SmitePriest",
		"gearSet":    "p3",
		"iterations": 3000,
		"async":      true,
	}

	started := callTool[simRunOutput](t, session, "sim_run", arguments)
	if started.Job == nil {
		t.Fatal("async run returned no job")
	}
	if started.Result != nil {
		t.Error("async run also returned a synchronous result")
	}
	if started.Job.Link == "" {
		t.Error("the job record carries no link back to the setup")
	}

	// The id is a content hash, so submitting the same run again joins it instead of starting a
	// second one.
	again := callTool[simRunOutput](t, session, "sim_run", arguments)
	if again.Job == nil || again.Job.ID != started.Job.ID {
		t.Errorf("resubmitting the same run produced a different job: %+v", again.Job)
	}

	finished := waitForJob(t, session, started.Job.ID)
	if finished.State != "done" {
		t.Fatalf("job ended as %s: %s", finished.State, finished.Error)
	}

	result := callTool[struct {
		Result resultSummary `json:"result"`
	}](t, session, "job_result", map[string]any{"id": started.Job.ID})
	if result.Result.Dps.Avg <= 0 {
		t.Errorf("dps = %v", result.Result.Dps.Avg)
	}
	if result.Result.Iterations != 3000 {
		t.Errorf("iterations = %d", result.Result.Iterations)
	}

	// Collecting a result must not consume it: a caller that lost the response can ask again.
	repeat := callTool[struct {
		Result resultSummary `json:"result"`
	}](t, session, "job_result", map[string]any{"id": started.Job.ID})
	if repeat.Result.Dps.Avg != result.Result.Dps.Avg {
		t.Error("reading a result twice gave different answers")
	}

	listed := callTool[struct {
		Jobs []jobRecord `json:"jobs"`
	}](t, session, "job_list", map[string]any{})
	var found bool
	for _, job := range listed.Jobs {
		if job.ID == started.Job.ID {
			found = true
		}
	}
	if !found {
		t.Error("the finished job is missing from job_list")
	}
}

func TestJobCancel(t *testing.T) {
	session := connect(t)

	started := callTool[simRunOutput](t, session, "sim_run", map[string]any{
		"spec":       "SmitePriest",
		"gearSet":    "p3",
		"iterations": 400000,
		"async":      true,
	})
	if started.Job == nil {
		t.Fatal("async run returned no job")
	}

	cancelled := callTool[struct {
		Job jobRecord `json:"job"`
	}](t, session, "job_cancel", map[string]any{"id": started.Job.ID})
	if cancelled.Job.State != "aborted" {
		t.Errorf("state after cancel = %s", cancelled.Job.State)
	}

	final := waitForJob(t, session, started.Job.ID)
	if final.State != "aborted" {
		t.Errorf("job ended as %s, want aborted", final.State)
	}

	if message := toolError(t, session, "job_result", map[string]any{"id": started.Job.ID}); message == "" {
		t.Error("expected collecting a cancelled job's result to explain why it cannot")
	}
}

func TestUnknownJob(t *testing.T) {
	session := connect(t)

	for _, tool := range []string{"job_status", "job_result", "job_cancel"} {
		if message := toolError(t, session, tool, map[string]any{"id": "0000000000000000"}); message == "" {
			t.Errorf("%s: expected an error naming the missing job", tool)
		}
	}
}

func TestStatWeights(t *testing.T) {
	session := connect(t)

	output := callTool[struct {
		Weights map[string]struct {
			Weight float64 `json:"weight"`
			Stdev  float64 `json:"stdev"`
			EP     float64 `json:"ep"`
		} `json:"weights"`
		Link string `json:"link"`
	}](t, session, "sim_stat_weights", map[string]any{
		"spec":       "SmitePriest",
		"gearSet":    "p3",
		"iterations": 100,
	})

	spellDamage, ok := output.Weights["SpellDamage"]
	if !ok {
		t.Fatalf("no spell damage weight in %v", output.Weights)
	}
	if spellDamage.Weight <= 0 {
		t.Errorf("spell damage is worth %v dps per point", spellDamage.Weight)
	}
	// Spell damage is the reference stat for a caster, so its EP is 1 by construction.
	if spellDamage.EP < 0.99 || spellDamage.EP > 1.01 {
		t.Errorf("spell damage EP = %v, want 1", spellDamage.EP)
	}
	if output.Link == "" {
		t.Error("no share link returned")
	}
}
