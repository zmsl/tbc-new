package jobs_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/wowsims/tbc/mcp/internal/engine"
	"github.com/wowsims/tbc/mcp/internal/engine/jobs"
	"github.com/wowsims/tbc/sim"
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
)

func init() {
	sim.RegisterAll()
}

func testRequest(t *testing.T, iterations int32, seed int64) *proto.RaidSimRequest {
	t.Helper()

	config := engine.FileConfig(filepath.Join("..", "..", "..", "..", "ui"))
	settings, _, err := config.BuildSettings(engine.SettingsRequest{
		Spec:    proto.Spec_SpecSmitePriest,
		GearSet: "p3",
	})
	if err != nil {
		t.Fatalf("build settings: %v", err)
	}

	request, err := core.BuildRaidSimRequest(settings, core.SimRequestOptions{Iterations: iterations, RandomSeed: seed})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	return request
}

func waitFor(t *testing.T, store jobs.Store, id string) jobs.Record {
	t.Helper()

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		record, err := store.Status(id)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if record.State != jobs.StateRunning {
			return record
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("job %s did not finish", id)
	return jobs.Record{}
}

// The id is what makes the store stateless: it names the computation, not the session.
func TestIDNamesTheComputation(t *testing.T) {
	first, err := jobs.ID("sim", testRequest(t, 100, 1))
	if err != nil {
		t.Fatalf("id: %v", err)
	}
	same, err := jobs.ID("sim", testRequest(t, 100, 1))
	if err != nil {
		t.Fatalf("id: %v", err)
	}
	if first != same {
		t.Error("two identical requests got different ids")
	}

	for name, request := range map[string]*proto.RaidSimRequest{
		"different seed":       testRequest(t, 100, 2),
		"different iterations": testRequest(t, 200, 1),
	} {
		other, err := jobs.ID("sim", request)
		if err != nil {
			t.Fatalf("id: %v", err)
		}
		if other == first {
			t.Errorf("a request with a %s got the same id", name)
		}
	}

	// The kind is part of the hash, so a stat-weights run and a sim of the same request do not
	// collide.
	weights, err := jobs.ID("statWeights", testRequest(t, 100, 1))
	if err != nil {
		t.Fatalf("id: %v", err)
	}
	if weights == first {
		t.Error("different kinds of run got the same id")
	}
}

func TestStartIsIdempotent(t *testing.T) {
	store := jobs.Store{Dir: t.TempDir()}
	request := testRequest(t, 2000, 1)

	first, err := store.Start("sim", "test", "", request)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	second, err := store.Start("sim", "test", "", request)
	if err != nil {
		t.Fatalf("start again: %v", err)
	}

	if first.ID != second.ID {
		t.Errorf("the same request started two jobs: %s and %s", first.ID, second.ID)
	}

	record := waitFor(t, store, first.ID)
	if record.State != jobs.StateDone {
		t.Fatalf("job ended as %s: %s", record.State, record.Error)
	}

	// Starting it once more after it finished returns the finished record rather than re-running.
	third, err := store.Start("sim", "test", "", request)
	if err != nil {
		t.Fatalf("start after finishing: %v", err)
	}
	if third.State != jobs.StateDone {
		t.Errorf("restarting a finished job put it back into %s", third.State)
	}
}

// Records live on disk, not in a process's memory: a store value that never saw the run can still
// answer for it, which is what makes a restart survivable.
func TestRecordsOutliveTheStoreThatMadeThem(t *testing.T) {
	dir := t.TempDir()
	request := testRequest(t, 1000, 3)

	started, err := jobs.Store{Dir: dir}.Start("sim", "test", "https://example.invalid/#x", request)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitFor(t, jobs.Store{Dir: dir}, started.ID)

	fresh := jobs.Store{Dir: dir}

	record, err := fresh.Status(started.ID)
	if err != nil {
		t.Fatalf("status from a fresh store: %v", err)
	}
	if record.State != jobs.StateDone {
		t.Fatalf("state = %s: %s", record.State, record.Error)
	}
	if record.Link != "https://example.invalid/#x" {
		t.Errorf("link = %q", record.Link)
	}

	result, err := fresh.Result(started.ID)
	if err != nil {
		t.Fatalf("result from a fresh store: %v", err)
	}
	if result.RaidMetrics.Dps.Avg <= 0 {
		t.Error("the stored result has no dps")
	}

	// Reading does not consume: the same store answers again.
	if _, err := fresh.Result(started.ID); err != nil {
		t.Errorf("second read: %v", err)
	}

	// And the request is stored too, so the record describes itself.
	stored, err := fresh.Request(started.ID)
	if err != nil {
		t.Fatalf("request from a fresh store: %v", err)
	}
	if stored.SimOptions.Iterations != 1000 {
		t.Errorf("stored request has %d iterations", stored.SimOptions.Iterations)
	}
}

func TestCancel(t *testing.T) {
	store := jobs.Store{Dir: t.TempDir()}

	started, err := store.Start("sim", "test", "", testRequest(t, 400000, 4))
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if _, err := store.Cancel(started.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	record := waitFor(t, store, started.ID)
	if record.State != jobs.StateAborted {
		t.Errorf("state = %s, want aborted", record.State)
	}
	if _, err := store.Result(started.ID); err == nil {
		t.Error("a cancelled job produced a result")
	}
}

func TestPruneRemovesOldRecords(t *testing.T) {
	dir := t.TempDir()
	store := jobs.Store{Dir: dir, TTL: time.Nanosecond}

	started, err := store.Start("sim", "test", "", testRequest(t, 100, 5))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitFor(t, store, started.ID)

	if err := store.Prune(); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if _, err := store.Status(started.ID); err == nil {
		t.Error("the record survived a prune with a one-nanosecond TTL")
	}

	// A generous TTL keeps them.
	keeper := jobs.Store{Dir: dir, TTL: time.Hour}
	kept, err := keeper.Start("sim", "test", "", testRequest(t, 100, 6))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitFor(t, keeper, kept.ID)
	if err := keeper.Prune(); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if _, err := keeper.Status(kept.ID); err != nil {
		t.Errorf("a fresh record was pruned: %v", err)
	}
}

func TestStatusReportsUnknownJobs(t *testing.T) {
	store := jobs.Store{Dir: t.TempDir()}

	if _, err := store.Status("nope"); err == nil {
		t.Error("expected an error for an unknown job")
	}
	records, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("an empty store listed %d jobs", len(records))
	}
}
