// Package poller periodically fetches tasks from every enabled provider,
// notifies about newly-seen items, and reports the merged snapshot back to
// the app.
package poller

import (
	"context"
	"log"
	"sort"
	"time"

	"koalmine/internal/notify"
	"koalmine/internal/providers"
	"koalmine/internal/store"
)

const defaultInterval = 5 * time.Minute

// Poller orchestrates polling. Zero value is ready to use.
type Poller struct {
	// OnUpdate is called after each poll with the merged, sorted (most
	// recently updated first) snapshot across all enabled providers.
	OnUpdate func(items []providers.TaskItem)

	stop chan struct{}
}

func New() *Poller {
	return &Poller{stop: make(chan struct{})}
}

// Run polls immediately, then again on every tick (interval read fresh from
// config each time, so a settings change takes effect on the next cycle),
// until ctx is cancelled or Stop is called.
func (p *Poller) Run(ctx context.Context) {
	p.pollOnce(ctx)

	for {
		select {
		case <-time.After(p.currentInterval()):
			p.pollOnce(ctx)
		case <-p.stop:
			return
		case <-ctx.Done():
			return
		}
	}
}

// PollNow triggers an immediate, out-of-cycle poll (used by "Actualizar ahora").
func (p *Poller) PollNow(ctx context.Context) {
	p.pollOnce(ctx)
}

// Stop ends Run's loop.
func (p *Poller) Stop() {
	close(p.stop)
}

func (p *Poller) currentInterval() time.Duration {
	cfg, err := store.Load()
	if err != nil || cfg.PollIntervalMinutes <= 0 {
		return defaultInterval
	}
	return time.Duration(cfg.PollIntervalMinutes) * time.Minute
}

func (p *Poller) pollOnce(ctx context.Context) {
	cfg, err := store.Load()
	if err != nil {
		log.Printf("poller: no se pudo cargar la configuración: %v", err)
		return
	}

	displayNames := map[string]string{}
	var all []providers.TaskItem

	for _, provider := range providers.List() {
		displayNames[provider.Name()] = provider.DisplayName()

		pc, ok := cfg.Providers[provider.Name()]
		if !ok || !pc.Enabled {
			continue
		}

		resolved, err := store.ResolveConfig(provider, provider.Name())
		if err != nil {
			log.Printf("poller: %s: %v", provider.Name(), err)
			continue
		}

		items, err := provider.FetchItems(ctx, resolved)
		if err != nil {
			log.Printf("poller: %s: %v", provider.Name(), err)
			continue
		}
		all = append(all, items...)
	}

	sort.Slice(all, func(i, j int) bool { return all[i].UpdatedAt.After(all[j].UpdatedAt) })

	p.notifyNewItems(all, displayNames)

	if p.OnUpdate != nil {
		p.OnUpdate(all)
	}
}

// notifyNewItems diffs items against the persisted seen-state and sends a
// notification for each one not seen before. Nothing is notified on the
// very first poll ever (there's no "new" relative to a state that doesn't
// exist yet) — it just seeds the state.
func (p *Poller) notifyNewItems(items []providers.TaskItem, displayNames map[string]string) {
	state, err := store.LoadSeenState()
	if err != nil {
		log.Printf("poller: no se pudo cargar el estado: %v", err)
		return
	}

	firstRun := len(state.IDs) == 0
	fresh := unseenItems(items, state.IDs)
	if len(fresh) == 0 {
		return
	}

	now := time.Now().Unix()
	for _, item := range fresh {
		state.IDs[item.ID] = now
	}

	// Nothing to notify about on the very first poll ever — there's no
	// "new" relative to a state that didn't exist yet, it just seeds it.
	if !firstRun {
		for _, item := range fresh {
			title := typeLabel(item.Type) + " · " + displayNames[item.Provider]
			if err := notify.Send(title, item.Title); err != nil {
				log.Printf("poller: no se pudo enviar la notificación: %v", err)
			}
		}
	}

	if err := store.SaveSeenState(state); err != nil {
		log.Printf("poller: no se pudo guardar el estado: %v", err)
	}
}

// unseenItems returns the items whose ID isn't a key in seenIDs.
func unseenItems(items []providers.TaskItem, seenIDs map[string]int64) []providers.TaskItem {
	var result []providers.TaskItem
	for _, item := range items {
		if _, ok := seenIDs[item.ID]; !ok {
			result = append(result, item)
		}
	}
	return result
}

func typeLabel(t providers.ItemType) string {
	switch t {
	case providers.ItemTypeIssue:
		return "Issue asignado"
	case providers.ItemTypePR:
		return "PR para revisar"
	case providers.ItemTypeMention:
		return "Mención"
	default:
		return string(t)
	}
}
