package core

import (
	"fmt"
	"sync"
	"testing"

	"github.com/wowsims/tbc/sim/core/proto"
)

// Every sim request carries the slice of the database it needs and adds it before reading,
// so the server adds entries while other requests are already reading. That used to be a
// fatal "concurrent map read and map write" that took the whole process down, because only
// the writers were locked.
//
// This covers the property that catches a careless fix as much as the original bug: copying
// the map without serialising writers lets two of them copy the same base and each drop the
// other's additions, which loses items silently instead of crashing loudly.
func TestAddToDatabaseConcurrent(t *testing.T) {
	const writers = 16
	const perWriter = 25

	// Ids well clear of any real item, so this cannot collide with a populated database.
	id := func(w, i int) int32 { return int32(900_000_000 + w*1_000 + i) }

	// Separate groups: the readers only stop once the writers are done, so waiting on one
	// group for both would deadlock.
	var writersWg, readersWg sync.WaitGroup
	stop := make(chan struct{})

	// Readers run throughout, indexing the maps the way the engine does.
	for r := 0; r < 4; r++ {
		readersWg.Add(1)
		go func() {
			defer readersWg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for w := 0; w < writers; w++ {
					_ = GetItemByID(id(w, 0))
				}
				for range ItemsByID {
					break
				}
			}
		}()
	}

	for w := 0; w < writers; w++ {
		writersWg.Add(1)
		go func(w int) {
			defer writersWg.Done()
			items := make([]*proto.SimItem, 0, perWriter)
			for i := 0; i < perWriter; i++ {
				items = append(items, &proto.SimItem{Id: id(w, i), Name: fmt.Sprintf("test-%d-%d", w, i)})
			}
			addToDatabase(&proto.SimDatabase{Items: items})
		}(w)
	}

	// Writers first; readers only stop once there is nothing left to race against.
	writersWg.Wait()
	close(stop)
	readersWg.Wait()

	for w := 0; w < writers; w++ {
		for i := 0; i < perWriter; i++ {
			if GetItemByID(id(w, i)) == nil {
				t.Fatalf("item %d added by writer %d is missing: a concurrent writer dropped it", id(w, i), w)
			}
		}
	}
}
