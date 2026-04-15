package outbox

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/martketplace-vkr/pkg/outbox"
)

const cmpName = "outbox"

type cmp struct {
	cfg          Config
	OutboxClient outbox.Outbox
}

func New(cfg Config, outboxClient outbox.Outbox) *cmp {
	return &cmp{
		cfg:          cfg,
		OutboxClient: outboxClient,
	}
}

func (c *cmp) Start(ctx context.Context) error {
	return c.OutboxClient.Run(ctx)
}

func (c *cmp) Stop(ctx context.Context) error {
	return c.OutboxClient.Shutdown(ctx)
}

func (c *cmp) Send(ctx context.Context, eventType string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = c.OutboxClient.CreateEvent(ctx, outbox.CreateEvent{
		Context:   ctx,
		EventType: eventType,
		Key:       uuid.NewString(),
		Payload:   body,
		Topics:    c.cfg.Topics,
	})
	return err
}

func (c *cmp) GetName() string {
	return cmpName
}

func (c *cmp) GetShutdownDelay() time.Duration {
	return time.Second
}

func (c *cmp) GetStartTimeout() time.Duration {
	return 5 * time.Second
}

func (c *cmp) GetStopTimeout() time.Duration {
	return 5 * time.Second
}
