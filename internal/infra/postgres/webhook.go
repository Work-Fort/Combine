package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/Work-Fort/Combine/internal/domain"
)

func scanWebhook(row interface{ Scan(dest ...any) error }) (*domain.Webhook, error) {
	var w domain.Webhook
	if err := row.Scan(
		&w.ID, &w.RepoID, &w.URL, &w.ContentType, &w.Active,
		&w.CreatedAt, &w.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &w, nil
}

func scanWebhooks(rows *sql.Rows) ([]*domain.Webhook, error) {
	var whs []*domain.Webhook
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		whs = append(whs, w)
	}
	return whs, rows.Err()
}

const webhookColumns = `id, repo_id, url, content_type, active, created_at, updated_at`

func scanWebhookEvent(row interface{ Scan(dest ...any) error }) (*domain.WebhookEvent, error) {
	var e domain.WebhookEvent
	if err := row.Scan(&e.ID, &e.WebhookID, &e.Event, &e.CreatedAt); err != nil {
		return nil, err
	}
	return &e, nil
}

func scanWebhookDelivery(row interface{ Scan(dest ...any) error }) (*domain.WebhookDelivery, error) {
	var d domain.WebhookDelivery
	var reqErr sql.NullString
	if err := row.Scan(
		&d.ID, &d.WebhookID, &d.Event, &d.RequestURL, &d.RequestMethod,
		&reqErr, &d.RequestHeaders, &d.RequestBody,
		&d.ResponseStatus, &d.ResponseHeaders, &d.ResponseBody,
		&d.CreatedAt,
	); err != nil {
		return nil, err
	}
	if reqErr.Valid {
		d.RequestError = reqErr.String
	}
	return &d, nil
}

const webhookDeliveryColumns = `id, webhook_id, event, request_url, request_method, request_error, request_headers, request_body, response_status, response_headers, response_body, created_at`

// --- Webhooks ---

func getWebhookByID(ctx context.Context, q querier, repoID, id int64) (*domain.Webhook, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+webhookColumns+` FROM webhooks WHERE repo_id = $1 AND id = $2`, repoID, id)
	w, err := scanWebhook(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: webhook id %d", domain.ErrNotFound, id)
	}
	return w, err
}

func listWebhooksByRepoID(ctx context.Context, q querier, repoID int64) ([]*domain.Webhook, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT `+webhookColumns+` FROM webhooks WHERE repo_id = $1`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWebhooks(rows)
}

func listWebhooksByRepoIDWhereEvent(ctx context.Context, q querier, repoID int64, events []int) ([]*domain.Webhook, error) {
	if len(events) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(events))
	args := make([]any, 0, len(events)+1)
	args = append(args, repoID)
	for i, e := range events {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, e)
	}

	query := `SELECT webhooks.id, webhooks.repo_id, webhooks.url, webhooks.content_type, webhooks.active, webhooks.created_at, webhooks.updated_at
		FROM webhooks
		INNER JOIN webhook_events ON webhooks.id = webhook_events.webhook_id
		WHERE webhooks.repo_id = $1 AND webhook_events.event IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWebhooks(rows)
}

func createWebhook(ctx context.Context, q querier, repoID int64, url string, contentType int, active bool) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx,
		`INSERT INTO webhooks (repo_id, url, content_type, active, updated_at)
		 VALUES ($1, $2, $3, $4, NOW()) RETURNING id`,
		repoID, url, contentType, active).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, fmt.Errorf("%w: webhook", domain.ErrAlreadyExists)
		}
		return 0, err
	}
	return id, nil
}

func updateWebhookByID(ctx context.Context, q querier, repoID, id int64, url string, contentType int, active bool) error {
	_, err := q.ExecContext(ctx,
		`UPDATE webhooks SET url = $1, content_type = $2, active = $3, updated_at = NOW()
		 WHERE repo_id = $4 AND id = $5`,
		url, contentType, active, repoID, id)
	return err
}

func deleteWebhookByID(ctx context.Context, q querier, id int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM webhooks WHERE id = $1`, id)
	return err
}

func deleteWebhookForRepoByID(ctx context.Context, q querier, repoID, id int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM webhooks WHERE repo_id = $1 AND id = $2`, repoID, id)
	return err
}

// --- Webhook Events ---

func getWebhookEventByID(ctx context.Context, q querier, id int64) (*domain.WebhookEvent, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id, webhook_id, event, created_at FROM webhook_events WHERE id = $1`, id)
	e, err := scanWebhookEvent(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: webhook event id %d", domain.ErrNotFound, id)
	}
	return e, err
}

func listWebhookEventsByWebhookID(ctx context.Context, q querier, webhookID int64) ([]*domain.WebhookEvent, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, webhook_id, event, created_at FROM webhook_events WHERE webhook_id = $1`, webhookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*domain.WebhookEvent
	for rows.Next() {
		e, err := scanWebhookEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func createWebhookEvents(ctx context.Context, q querier, webhookID int64, events []int) error {
	for _, event := range events {
		_, err := q.ExecContext(ctx,
			`INSERT INTO webhook_events (webhook_id, event) VALUES ($1, $2)`,
			webhookID, event)
		if err != nil {
			return err
		}
	}
	return nil
}

func deleteWebhookEventsByID(ctx context.Context, q querier, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	_, err := q.ExecContext(ctx,
		`DELETE FROM webhook_events WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	return err
}

// --- Webhook Deliveries ---

func getWebhookDeliveryByID(ctx context.Context, q querier, webhookID int64, id uuid.UUID) (*domain.WebhookDelivery, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+webhookDeliveryColumns+` FROM webhook_deliveries WHERE webhook_id = $1 AND id = $2`,
		webhookID, id.String())
	d, err := scanWebhookDelivery(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: webhook delivery %s", domain.ErrNotFound, id)
	}
	return d, err
}

func getWebhookDeliveriesByWebhookID(ctx context.Context, q querier, webhookID int64) ([]*domain.WebhookDelivery, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT `+webhookDeliveryColumns+` FROM webhook_deliveries WHERE webhook_id = $1`, webhookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []*domain.WebhookDelivery
	for rows.Next() {
		d, err := scanWebhookDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

func listWebhookDeliveriesByWebhookID(ctx context.Context, q querier, webhookID int64) ([]*domain.WebhookDelivery, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, webhook_id, event, request_url, request_method, request_error, request_headers, request_body, response_status, response_headers, response_body, created_at
		 FROM webhook_deliveries WHERE webhook_id = $1`, webhookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []*domain.WebhookDelivery
	for rows.Next() {
		d, err := scanWebhookDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

func createWebhookDelivery(ctx context.Context, q querier, id uuid.UUID, webhookID int64, event int, url, method string, requestError error, requestHeaders, requestBody string, responseStatus int, responseHeaders, responseBody string) error {
	var reqErr string
	if requestError != nil {
		reqErr = requestError.Error()
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO webhook_deliveries (id, webhook_id, event, request_url, request_method, request_error, request_headers, request_body, response_status, response_headers, response_body)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		id.String(), webhookID, event, url, method, reqErr, requestHeaders, requestBody, responseStatus, responseHeaders, responseBody)
	return err
}

func deleteWebhookDeliveryByID(ctx context.Context, q querier, webhookID int64, id uuid.UUID) error {
	_, err := q.ExecContext(ctx,
		`DELETE FROM webhook_deliveries WHERE webhook_id = $1 AND id = $2`, webhookID, id.String())
	return err
}

// Store methods.

func (s *Store) GetWebhookByID(ctx context.Context, repoID, id int64) (*domain.Webhook, error) {
	return getWebhookByID(ctx, s.q(), repoID, id)
}

func (s *Store) ListWebhooksByRepoID(ctx context.Context, repoID int64) ([]*domain.Webhook, error) {
	return listWebhooksByRepoID(ctx, s.q(), repoID)
}

func (s *Store) ListWebhooksByRepoIDWhereEvent(ctx context.Context, repoID int64, events []int) ([]*domain.Webhook, error) {
	return listWebhooksByRepoIDWhereEvent(ctx, s.q(), repoID, events)
}

func (s *Store) CreateWebhook(ctx context.Context, repoID int64, url string, contentType int, active bool) (int64, error) {
	return createWebhook(ctx, s.q(), repoID, url, contentType, active)
}

func (s *Store) UpdateWebhookByID(ctx context.Context, repoID, id int64, url string, contentType int, active bool) error {
	return updateWebhookByID(ctx, s.q(), repoID, id, url, contentType, active)
}

func (s *Store) DeleteWebhookByID(ctx context.Context, id int64) error {
	return deleteWebhookByID(ctx, s.q(), id)
}

func (s *Store) DeleteWebhookForRepoByID(ctx context.Context, repoID, id int64) error {
	return deleteWebhookForRepoByID(ctx, s.q(), repoID, id)
}

func (s *Store) GetWebhookEventByID(ctx context.Context, id int64) (*domain.WebhookEvent, error) {
	return getWebhookEventByID(ctx, s.q(), id)
}

func (s *Store) ListWebhookEventsByWebhookID(ctx context.Context, webhookID int64) ([]*domain.WebhookEvent, error) {
	return listWebhookEventsByWebhookID(ctx, s.q(), webhookID)
}

func (s *Store) CreateWebhookEvents(ctx context.Context, webhookID int64, events []int) error {
	return createWebhookEvents(ctx, s.q(), webhookID, events)
}

func (s *Store) DeleteWebhookEventsByID(ctx context.Context, ids []int64) error {
	return deleteWebhookEventsByID(ctx, s.q(), ids)
}

func (s *Store) GetWebhookDeliveryByID(ctx context.Context, webhookID int64, id uuid.UUID) (*domain.WebhookDelivery, error) {
	return getWebhookDeliveryByID(ctx, s.q(), webhookID, id)
}

func (s *Store) GetWebhookDeliveriesByWebhookID(ctx context.Context, webhookID int64) ([]*domain.WebhookDelivery, error) {
	return getWebhookDeliveriesByWebhookID(ctx, s.q(), webhookID)
}

func (s *Store) ListWebhookDeliveriesByWebhookID(ctx context.Context, webhookID int64) ([]*domain.WebhookDelivery, error) {
	return listWebhookDeliveriesByWebhookID(ctx, s.q(), webhookID)
}

func (s *Store) CreateWebhookDelivery(ctx context.Context, id uuid.UUID, webhookID int64, event int, url, method string, requestError error, requestHeaders, requestBody string, responseStatus int, responseHeaders, responseBody string) error {
	return createWebhookDelivery(ctx, s.q(), id, webhookID, event, url, method, requestError, requestHeaders, requestBody, responseStatus, responseHeaders, responseBody)
}

func (s *Store) DeleteWebhookDeliveryByID(ctx context.Context, webhookID int64, id uuid.UUID) error {
	return deleteWebhookDeliveryByID(ctx, s.q(), webhookID, id)
}

// txStore methods.

func (ts *txStore) GetWebhookByID(ctx context.Context, repoID, id int64) (*domain.Webhook, error) {
	return getWebhookByID(ctx, ts.q(), repoID, id)
}

func (ts *txStore) ListWebhooksByRepoID(ctx context.Context, repoID int64) ([]*domain.Webhook, error) {
	return listWebhooksByRepoID(ctx, ts.q(), repoID)
}

func (ts *txStore) ListWebhooksByRepoIDWhereEvent(ctx context.Context, repoID int64, events []int) ([]*domain.Webhook, error) {
	return listWebhooksByRepoIDWhereEvent(ctx, ts.q(), repoID, events)
}

func (ts *txStore) CreateWebhook(ctx context.Context, repoID int64, url string, contentType int, active bool) (int64, error) {
	return createWebhook(ctx, ts.q(), repoID, url, contentType, active)
}

func (ts *txStore) UpdateWebhookByID(ctx context.Context, repoID, id int64, url string, contentType int, active bool) error {
	return updateWebhookByID(ctx, ts.q(), repoID, id, url, contentType, active)
}

func (ts *txStore) DeleteWebhookByID(ctx context.Context, id int64) error {
	return deleteWebhookByID(ctx, ts.q(), id)
}

func (ts *txStore) DeleteWebhookForRepoByID(ctx context.Context, repoID, id int64) error {
	return deleteWebhookForRepoByID(ctx, ts.q(), repoID, id)
}

func (ts *txStore) GetWebhookEventByID(ctx context.Context, id int64) (*domain.WebhookEvent, error) {
	return getWebhookEventByID(ctx, ts.q(), id)
}

func (ts *txStore) ListWebhookEventsByWebhookID(ctx context.Context, webhookID int64) ([]*domain.WebhookEvent, error) {
	return listWebhookEventsByWebhookID(ctx, ts.q(), webhookID)
}

func (ts *txStore) CreateWebhookEvents(ctx context.Context, webhookID int64, events []int) error {
	return createWebhookEvents(ctx, ts.q(), webhookID, events)
}

func (ts *txStore) DeleteWebhookEventsByID(ctx context.Context, ids []int64) error {
	return deleteWebhookEventsByID(ctx, ts.q(), ids)
}

func (ts *txStore) GetWebhookDeliveryByID(ctx context.Context, webhookID int64, id uuid.UUID) (*domain.WebhookDelivery, error) {
	return getWebhookDeliveryByID(ctx, ts.q(), webhookID, id)
}

func (ts *txStore) GetWebhookDeliveriesByWebhookID(ctx context.Context, webhookID int64) ([]*domain.WebhookDelivery, error) {
	return getWebhookDeliveriesByWebhookID(ctx, ts.q(), webhookID)
}

func (ts *txStore) ListWebhookDeliveriesByWebhookID(ctx context.Context, webhookID int64) ([]*domain.WebhookDelivery, error) {
	return listWebhookDeliveriesByWebhookID(ctx, ts.q(), webhookID)
}

func (ts *txStore) CreateWebhookDelivery(ctx context.Context, id uuid.UUID, webhookID int64, event int, url, method string, requestError error, requestHeaders, requestBody string, responseStatus int, responseHeaders, responseBody string) error {
	return createWebhookDelivery(ctx, ts.q(), id, webhookID, event, url, method, requestError, requestHeaders, requestBody, responseStatus, responseHeaders, responseBody)
}

func (ts *txStore) DeleteWebhookDeliveryByID(ctx context.Context, webhookID int64, id uuid.UUID) error {
	return deleteWebhookDeliveryByID(ctx, ts.q(), webhookID, id)
}
