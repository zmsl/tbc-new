// Package jobs runs long simulations without making the server stateful.
//
// A job is named by a content hash of the request it runs, and its record lives in a directory
// on disk rather than in this process's memory. Three things follow, and they are the whole
// point:
//
//   - submitting the same request twice names the same job, so a retry joins the run already in
//     progress instead of starting a second one;
//   - any instance can answer for any id, and a restart loses nothing that was finished;
//   - a result stays readable until it is pruned, rather than being consumed by the first poll.
//
// Determinism is what makes the naming honest: the seed is part of the request, so a given id
// always describes the same computation and re-running it is indistinguishable from resuming it.
package jobs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
	"github.com/wowsims/tbc/sim/core/simsignals"
	googleProto "google.golang.org/protobuf/proto"
)

// State is where a job has got to.
type State string

const (
	StateRunning State = "running"
	StateDone    State = "done"
	StateFailed  State = "failed"
	StateAborted State = "aborted"
	// StateStale means the process that was running the job went away. Re-submitting the same
	// request starts it again, which is equivalent because the run is deterministic.
	StateStale State = "stale"
)

// A record whose heartbeat is older than this is presumed abandoned.
const heartbeatTimeout = 60 * time.Second

// How often a running job writes its progress. Frequent enough to poll usefully, rare enough not
// to spend the run writing files.
const heartbeatInterval = 500 * time.Millisecond

// DefaultTTL is how long finished jobs stay readable.
const DefaultTTL = 24 * time.Hour

var ErrNotFound = errors.New("no such job")

// Record is a job's status, as stored and as reported.
type Record struct {
	ID                  string    `json:"id" jsonschema:"the job id; a content hash of the request, so the same request always has the same id"`
	Kind                string    `json:"kind" jsonschema:"what is being run, e.g. sim or statWeights"`
	State               State     `json:"state" jsonschema:"running, done, failed, aborted or stale"`
	Label               string    `json:"label,omitempty" jsonschema:"a human-readable description of the run"`
	Link                string    `json:"link,omitempty" jsonschema:"share link for the setup being simulated"`
	CompletedIterations int32     `json:"completedIterations"`
	TotalIterations     int32     `json:"totalIterations"`
	StartedAt           time.Time `json:"startedAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
	PID                 int       `json:"pid" jsonschema:"the process running it, for diagnosis"`
	Error               string    `json:"error,omitempty" jsonschema:"why the job failed, when it did"`
}

// PercentComplete is progress as a percentage, or zero when the total is unknown.
func (r Record) PercentComplete() float64 {
	if r.TotalIterations <= 0 {
		return 0
	}
	return float64(r.CompletedIterations) / float64(r.TotalIterations) * 100
}

// Store is a directory of job records.
type Store struct {
	Dir string
	TTL time.Duration
}

// ID is the name a request will be filed under. Marshalling is deterministic, and the request
// carries its own seed and iteration count, so equal ids mean equal computations.
func ID(kind string, request googleProto.Message) (string, error) {
	encoded, err := googleProto.MarshalOptions{Deterministic: true}.Marshal(request)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte(kind+"\x00"), encoded...))
	return hex.EncodeToString(sum[:16]), nil
}

func (s Store) path(id string, name string) string {
	return filepath.Join(s.Dir, id, name)
}

// Start files a request and begins running it, or returns the existing record if this request is
// already known. It never starts a second run of the same request.
func (s Store) Start(kind, label, link string, request *proto.RaidSimRequest) (Record, error) {
	id, err := ID(kind, request)
	if err != nil {
		return Record{}, err
	}

	if existing, err := s.Status(id); err == nil {
		if existing.State != StateStale {
			return existing, nil
		}
		// The previous owner died. Clear the record and run it again: same request, same seed,
		// same answer.
		if err := os.RemoveAll(filepath.Join(s.Dir, id)); err != nil {
			return Record{}, err
		}
	} else if !errors.Is(err, ErrNotFound) {
		return Record{}, err
	}

	if err := os.MkdirAll(filepath.Join(s.Dir, id), 0o755); err != nil {
		return Record{}, err
	}

	record := Record{
		ID:              id,
		Kind:            kind,
		State:           StateRunning,
		Label:           label,
		Link:            link,
		TotalIterations: request.SimOptions.GetIterations(),
		StartedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
		PID:             os.Getpid(),
	}

	// O_EXCL is the claim: whoever creates status.json owns the run. A second caller -- another
	// goroutine here, or another process sharing the directory -- loses the race and reads the
	// winner's record instead of starting a duplicate.
	file, err := os.OpenFile(s.path(id, "status.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return s.Status(id)
		}
		return Record{}, err
	}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		file.Close()
		return Record{}, err
	}
	if _, err := file.Write(encoded); err != nil {
		file.Close()
		return Record{}, err
	}
	file.Close()

	// The request is stored so the record describes itself: any instance can say what a job is,
	// and re-run it if the one that started it went away.
	requestBytes, err := googleProto.Marshal(request)
	if err != nil {
		return Record{}, err
	}
	if err := os.WriteFile(s.path(id, "request.binpb"), requestBytes, 0o644); err != nil {
		return Record{}, err
	}

	go s.run(record, request)
	return record, nil
}

func (s Store) run(record Record, request *proto.RaidSimRequest) {
	progress := make(chan *proto.ProgressMetrics, 32)
	core.RunRaidSimConcurrentAsync(request, progress, record.ID)

	lastWrite := time.Now()
	for update := range progress {
		if update == nil {
			break
		}

		if update.FinalRaidResult != nil {
			s.finish(record, update.FinalRaidResult)
			return
		}

		record.CompletedIterations = update.CompletedIterations
		if update.TotalIterations > 0 {
			record.TotalIterations = update.TotalIterations
		}

		// A cancel written by another process cannot reach simsignals directly, so the sentinel
		// is checked on the way past.
		if s.cancelled(record.ID) {
			simsignals.AbortById(record.ID)
		}

		if time.Since(lastWrite) >= heartbeatInterval {
			record.UpdatedAt = time.Now().UTC()
			_ = s.write(record)
			lastWrite = time.Now()
		}
	}

	// The channel closed without a final result, which means the runner gave up on us.
	record.State = StateFailed
	record.Error = "the simulation stopped without producing a result"
	record.UpdatedAt = time.Now().UTC()
	_ = s.write(record)
}

func (s Store) finish(record Record, result *proto.RaidSimResult) {
	record.UpdatedAt = time.Now().UTC()
	record.State = StateDone

	if result.Error != nil {
		record.Error = result.Error.Message
		record.State = StateFailed
		if result.Error.Type == proto.ErrorOutcomeType_ErrorOutcomeAborted {
			record.State = StateAborted
			record.Error = "cancelled"
		}
	}

	if record.State == StateDone {
		record.CompletedIterations = record.TotalIterations
		if encoded, err := googleProto.Marshal(result); err == nil {
			_ = os.WriteFile(s.path(record.ID, "result.binpb"), encoded, 0o644)
		} else {
			record.State = StateFailed
			record.Error = err.Error()
		}
	}

	_ = s.write(record)
}

func (s Store) write(record Record) error {
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	// Written whole and renamed into place, so a poll never reads half a record.
	temporary := s.path(record.ID, "status.json.tmp")
	if err := os.WriteFile(temporary, encoded, 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, s.path(record.ID, "status.json"))
}

// Status reports a job's record, marking it stale if the process running it stopped updating.
func (s Store) Status(id string) (Record, error) {
	data, err := os.ReadFile(s.path(id, "status.json"))
	if os.IsNotExist(err) {
		return Record{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return Record{}, err
	}

	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, err
	}

	if record.State == StateRunning && time.Since(record.UpdatedAt) > heartbeatTimeout {
		record.State = StateStale
	}
	return record, nil
}

// Result returns a finished job's result. It stays readable until the record is pruned, so a
// caller that lost a response can ask again.
func (s Store) Result(id string) (*proto.RaidSimResult, error) {
	record, err := s.Status(id)
	if err != nil {
		return nil, err
	}
	if record.State != StateDone {
		return nil, fmt.Errorf("job %s is %s", id, record.State)
	}

	data, err := os.ReadFile(s.path(id, "result.binpb"))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: result for %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}

	result := &proto.RaidSimResult{}
	if err := googleProto.Unmarshal(data, result); err != nil {
		return nil, err
	}
	return result, nil
}

// Request returns the request a job was filed with.
func (s Store) Request(id string) (*proto.RaidSimRequest, error) {
	data, err := os.ReadFile(s.path(id, "request.binpb"))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: request for %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}

	request := &proto.RaidSimRequest{}
	if err := googleProto.Unmarshal(data, request); err != nil {
		return nil, err
	}
	return request, nil
}

// Cancel stops a running job. The sentinel file is what lets a cancel written by one process
// reach a run owned by another; the direct abort is what makes it immediate in this one.
func (s Store) Cancel(id string) (Record, error) {
	record, err := s.Status(id)
	if err != nil {
		return Record{}, err
	}
	if record.State != StateRunning {
		return record, nil
	}

	if err := os.WriteFile(s.path(id, "cancel"), []byte(time.Now().UTC().Format(time.RFC3339)), 0o644); err != nil {
		return Record{}, err
	}
	simsignals.AbortById(id)

	record.State = StateAborted
	return record, nil
}

func (s Store) cancelled(id string) bool {
	_, err := os.Stat(s.path(id, "cancel"))
	return err == nil
}

// List reports every known job, most recently started first.
func (s Store) List() ([]Record, error) {
	entries, err := os.ReadDir(s.Dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var records []Record
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		record, err := s.Status(entry.Name())
		if err != nil {
			continue
		}
		records = append(records, record)
	}

	sort.Slice(records, func(i, j int) bool { return records[i].StartedAt.After(records[j].StartedAt) })
	return records, nil
}

// Prune deletes records older than the store's TTL. Results are kept until then rather than
// being deleted when read, so asking twice is allowed.
func (s Store) Prune() error {
	ttl := s.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	records, err := s.List()
	if err != nil {
		return err
	}
	for _, record := range records {
		if time.Since(record.StartedAt) > ttl {
			if err := os.RemoveAll(filepath.Join(s.Dir, record.ID)); err != nil {
				return err
			}
		}
	}
	return nil
}
