package relay

import (
	"context"
	"time"

	"github.com/ploenk42/striker/striker/internal/fleet"
	"github.com/ploenk42/striker/striker/internal/hc"
)

// Relay is the loop striker serve runs: every tick it reads the requests of
// the relayed agents, carries out the new ones and keeps the watches going.
type Relay struct {
	C hc.Client
	// Herdr runs herdr-app commands (coord.Start opens spaces with it).
	Herdr func(...string) ([]byte, error)
	// Fleet is loaded once per tick, so edits to fleet.json count at once.
	Fleet func() fleet.Fleet
	// Briefs is the folder of the brief templates.
	Briefs string
	L      *Ledger
	// W watches the task agents; nil: no watches (tests).
	W *Watcher
	// Logf reports what cannot be answered to anyone; may be nil.
	Logf func(format string, a ...any)
}

// Tick reads the status files once and handles each new request in file
// order: a refused one is recorded and answered, the rest carried out. Then
// it starts watches for open ledger entries. The error is a failed ledger
// save, after which the tick stops (nothing is done twice).
func (r *Relay) Tick(ctx context.Context) error {
	f := r.Fleet()
	for _, file := range Read(r.C, f) {
		for _, req := range file.Requests {
			if ctx.Err() != nil {
				return nil
			}
			// Filtered one request at a time so refusals and actions keep the
			// file's order; a repeated id finds its key in the ledger.
			one := file
			one.Requests = []Request{req}
			keep, drop := Filter(f, r.L, []File{one})
			for _, d := range drop {
				if err := Reject(r.C, r.L, f, d); err != nil {
					return err
				}
			}
			for _, it := range keep {
				if err := Act(r.C, r.Herdr, f, r.Briefs, r.L, it.File, it.Request); err != nil {
					return err
				}
			}
		}
	}
	if r.W != nil {
		r.W.Start(ctx)
	}
	return nil
}

// Run resumes the watches of the ledger, then ticks at once and every
// `every` until ctx ends; it returns after the watches have ended.
func (r *Relay) Run(ctx context.Context, every time.Duration) {
	if r.W != nil {
		r.W.Start(ctx)
		defer r.W.Wait()
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if err := r.Tick(ctx); err != nil && r.Logf != nil {
			r.Logf("relay: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
