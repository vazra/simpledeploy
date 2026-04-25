package audit

import "context"

type Ctx struct {
	ActorUserID *int64
	ActorName   string
	ActorSource string
	IP          string
	RequestID   string
}

type ctxKey struct{}

func With(ctx context.Context, c Ctx) context.Context {
	return context.WithValue(ctx, ctxKey{}, c)
}

func From(ctx context.Context) Ctx {
	v, _ := ctx.Value(ctxKey{}).(Ctx)
	if v.ActorSource == "" {
		v.ActorSource = "system"
	}
	return v
}
