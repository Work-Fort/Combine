package backend

import (
	"context"
	"encoding/json"

	"charm.land/log/v2"
	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/webhook"
	"github.com/google/uuid"
)

// CreateWebhook creates a webhook for a repository.
func (b *Backend) CreateWebhook(ctx context.Context, repo *domain.Repo, url string, contentType webhook.ContentType, secret string, events []webhook.Event, active bool) error {
	// Validate webhook URL to prevent SSRF attacks
	if err := webhook.ValidateWebhookURL(url); err != nil {
		return err //nolint:wrapcheck
	}

	return b.store.Transaction(ctx, func(tx domain.Store) error {
		lastID, err := tx.CreateWebhook(ctx, repo.ID, url, secret, int(contentType), active)
		if err != nil {
			return err
		}

		evs := make([]int, len(events))
		for i, e := range events {
			evs[i] = int(e)
		}
		return tx.CreateWebhookEvents(ctx, lastID, evs)
	})
}

// Webhook returns a webhook for a repository.
func (b *Backend) Webhook(ctx context.Context, repo *domain.Repo, id int64) (webhook.Hook, error) {
	var wh webhook.Hook
	if err := b.store.Transaction(ctx, func(tx domain.Store) error {
		h, err := tx.GetWebhookByID(ctx, repo.ID, id)
		if err != nil {
			return err
		}
		events, err := tx.ListWebhookEventsByWebhookID(ctx, id)
		if err != nil {
			return err
		}

		wh = webhook.Hook{
			Webhook:     *h,
			ContentType: webhook.ContentType(h.ContentType), //nolint:gosec
			Events:      make([]webhook.Event, len(events)),
		}
		for i, e := range events {
			wh.Events[i] = webhook.Event(e.Event)
		}

		return nil
	}); err != nil {
		return webhook.Hook{}, err
	}

	return wh, nil
}

// ListWebhooks lists webhooks for a repository.
func (b *Backend) ListWebhooks(ctx context.Context, repo *domain.Repo) ([]webhook.Hook, error) {
	var webhooks []*domain.Webhook
	webhookEvents := map[int64][]*domain.WebhookEvent{}
	if err := b.store.Transaction(ctx, func(tx domain.Store) error {
		var err error
		webhooks, err = tx.ListWebhooksByRepoID(ctx, repo.ID)
		if err != nil {
			return err
		}

		for _, h := range webhooks {
			events, err := tx.ListWebhookEventsByWebhookID(ctx, h.ID)
			if err != nil {
				return err
			}
			webhookEvents[h.ID] = events
		}

		return nil
	}); err != nil {
		return nil, err
	}

	hooks := make([]webhook.Hook, len(webhooks))
	for i, h := range webhooks {
		events := make([]webhook.Event, len(webhookEvents[h.ID]))
		for j, e := range webhookEvents[h.ID] {
			events[j] = webhook.Event(e.Event)
		}

		hooks[i] = webhook.Hook{
			Webhook:     *h,
			ContentType: webhook.ContentType(h.ContentType), //nolint:gosec
			Events:      events,
		}
	}

	return hooks, nil
}

// UpdateWebhook updates a webhook.
func (b *Backend) UpdateWebhook(ctx context.Context, repo *domain.Repo, id int64, url string, contentType webhook.ContentType, secret string, updatedEvents []webhook.Event, active bool) error {
	// Validate webhook URL to prevent SSRF attacks
	if err := webhook.ValidateWebhookURL(url); err != nil {
		return err
	}

	return b.store.Transaction(ctx, func(tx domain.Store) error {
		if err := tx.UpdateWebhookByID(ctx, repo.ID, id, url, secret, int(contentType), active); err != nil {
			return err
		}

		currentEvents, err := tx.ListWebhookEventsByWebhookID(ctx, id)
		if err != nil {
			return err
		}

		// Delete events that are no longer in the list.
		toBeDeleted := make([]int64, 0)
		for _, e := range currentEvents {
			found := false
			for _, ne := range updatedEvents {
				if int(ne) == e.Event {
					found = true
					break
				}
			}
			if !found {
				toBeDeleted = append(toBeDeleted, e.ID)
			}
		}

		if err := tx.DeleteWebhookEventsByID(ctx, toBeDeleted); err != nil {
			return err
		}

		// Prune events that are already in the list.
		newEvents := make([]int, 0)
		for _, e := range updatedEvents {
			found := false
			for _, ne := range currentEvents {
				if int(e) == ne.Event {
					found = true
					break
				}
			}
			if !found {
				newEvents = append(newEvents, int(e))
			}
		}

		return tx.CreateWebhookEvents(ctx, id, newEvents)
	})
}

// DeleteWebhook deletes a webhook for a repository.
func (b *Backend) DeleteWebhook(ctx context.Context, repo *domain.Repo, id int64) error {
	return b.store.Transaction(ctx, func(tx domain.Store) error {
		_, err := tx.GetWebhookByID(ctx, repo.ID, id)
		if err != nil {
			return err
		}
		return tx.DeleteWebhookForRepoByID(ctx, repo.ID, id)
	})
}

// ListWebhookDeliveries lists webhook deliveries for a webhook.
func (b *Backend) ListWebhookDeliveries(ctx context.Context, id int64) ([]webhook.Delivery, error) {
	deliveries, err := b.store.ListWebhookDeliveriesByWebhookID(ctx, id)
	if err != nil {
		return nil, err
	}

	ds := make([]webhook.Delivery, len(deliveries))
	for i, d := range deliveries {
		ds[i] = webhook.Delivery{
			WebhookDelivery: *d,
			Event:           webhook.Event(d.Event),
		}
	}

	return ds, nil
}

// RedeliverWebhookDelivery redelivers a webhook delivery.
func (b *Backend) RedeliverWebhookDelivery(ctx context.Context, repo *domain.Repo, id int64, delID uuid.UUID) error {
	var delivery *domain.WebhookDelivery
	var wh *domain.Webhook
	if err := b.store.Transaction(ctx, func(tx domain.Store) error {
		var err error
		wh, err = tx.GetWebhookByID(ctx, repo.ID, id)
		if err != nil {
			log.Errorf("error getting webhook: %v", err)
			return err
		}

		delivery, err = tx.GetWebhookDeliveryByID(ctx, id, delID)
		if err != nil {
			return err
		}

		return nil
	}); err != nil {
		return err
	}

	log.Infof("redelivering webhook delivery %s for webhook %d\n\n%s\n\n", delID, id, delivery.RequestBody)

	var payload json.RawMessage
	if err := json.Unmarshal([]byte(delivery.RequestBody), &payload); err != nil {
		log.Errorf("error unmarshaling webhook payload: %v", err)
		return err
	}

	return webhook.SendWebhook(ctx, *wh, webhook.Event(delivery.Event), payload)
}

// WebhookDelivery returns a webhook delivery.
func (b *Backend) WebhookDelivery(ctx context.Context, webhookID int64, id uuid.UUID) (webhook.Delivery, error) {
	d, err := b.store.GetWebhookDeliveryByID(ctx, webhookID, id)
	if err != nil {
		return webhook.Delivery{}, err
	}

	return webhook.Delivery{
		WebhookDelivery: *d,
		Event:           webhook.Event(d.Event),
	}, nil
}
