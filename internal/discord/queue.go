package discord

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

// Result is the outcome of a background delivery.
type Result struct {
	Kind    string
	Ref     string
	Message Message
	Err     error
}

// Queue delivers notices off the request path.
// Post methods return as soon as the notice is queued. A Discord failure,
// including a spent retry budget, is logged and reported to After.
// It is never returned to the caller of Post.
type Queue struct {
	client *Client
	jobs   chan job
	// After runs once delivery finishes, including when Err is set.
	// It may be nil. It must not block for long.
	After func(Result)
}

type job struct {
	kind       string
	ref        string
	completion Completion
	saga       SagaNotice
	level      LevelNotice
}

// NewQueue starts one worker so deliveries stay in order and share the rate-limit budget.
func NewQueue(client *Client) *Queue {
	q := &Queue{client: client, jobs: make(chan job, 32)}
	go q.loop()
	return q
}

// PostCompletion queues a completion. Ref on the result is ProgressID.
func (q *Queue) PostCompletion(_ context.Context, n Completion) (Message, error) {
	if err := q.ready(); err != nil {
		return Message{}, err
	}
	n.Image = append([]byte(nil), n.Image...)
	q.enqueue(job{kind: KindCompletion, ref: n.ProgressID, completion: n})
	return Message{Content: CompletionText(n.Character, n.Title), Accepted: true}, nil
}

// PostSaga queues an announcement, version note, or retirement.
func (q *Queue) PostSaga(_ context.Context, n SagaNotice) (Message, error) {
	if err := q.ready(); err != nil {
		return Message{}, err
	}
	n.Image = append([]byte(nil), n.Image...)
	q.enqueue(job{kind: n.Kind, saga: n})
	return Message{Accepted: true}, nil
}

// PostLevel queues a milestone level-up. Levels that are not milestones are ignored.
func (q *Queue) PostLevel(_ context.Context, n LevelNotice) (Message, error) {
	if err := q.ready(); err != nil {
		return Message{}, err
	}
	if !Milestone(n.Level) {
		return Message{}, nil
	}
	q.enqueue(job{kind: KindLevel, level: n})
	return Message{Accepted: true}, nil
}

func (q *Queue) ready() error {
	if q == nil || q.client == nil || strings.TrimSpace(q.client.WebhookURL) == "" {
		return ErrNotConfigured
	}
	return nil
}

func (q *Queue) enqueue(j job) {
	select {
	case q.jobs <- j:
	default:
		go q.run(j)
	}
}

func (q *Queue) loop() {
	for j := range q.jobs {
		q.run(j)
	}
}

func (q *Queue) run(j job) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("discord", "kind", j.kind, "err", rec)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var (
		msg Message
		err error
	)
	switch j.kind {
	case KindCompletion:
		msg, err = q.client.PostCompletion(ctx, j.completion)
	case KindAnnounced, KindUpdated, KindRetired:
		msg, err = q.client.PostSaga(ctx, j.saga)
	case KindLevel:
		msg, err = q.client.PostLevel(ctx, j.level)
	default:
		err = errUnknownKind(j.kind)
	}
	res := Result{Kind: j.kind, Ref: j.ref, Message: msg, Err: err}
	if err != nil {
		slog.Error("discord", "kind", j.kind, "err", err)
	}
	if q.After != nil {
		q.After(res)
	}
}

type kindError string

func (e kindError) Error() string { return "unknown discord notice " + string(e) }

func errUnknownKind(kind string) error { return kindError(kind) }
