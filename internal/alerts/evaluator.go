package alerts

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/vazra/simpledeploy/internal/metrics"
	"github.com/vazra/simpledeploy/internal/store"
)

type MetricQuerier interface {
	QueryMetrics(appID *int64, tier string, from, to time.Time) ([]metrics.MetricPoint, error)
}

type AlertStoreReader interface {
	ListActiveAlertRules() ([]store.AlertRule, error)
	GetActiveAlert(ruleID int64) (*store.AlertHistory, error)
	CreateAlertHistory(ruleID int64, value float64) (*store.AlertHistory, error)
	ResolveAlert(historyID int64) error
	GetWebhook(id int64) (*store.Webhook, error)
}

type AppLookup interface {
	GetAppByID(id int64) (*store.App, error)
}

type Evaluator struct {
	store      AlertStoreReader
	appLookup  AppLookup
	metrics    MetricQuerier
	dispatcher *WebhookDispatcher
}

func NewEvaluator(s AlertStoreReader, appLookup AppLookup, mq MetricQuerier, dispatcher *WebhookDispatcher) *Evaluator {
	return &Evaluator{
		store:      s,
		appLookup:  appLookup,
		metrics:    mq,
		dispatcher: dispatcher,
	}
}

func (e *Evaluator) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.EvaluateOnce(ctx); err != nil {
				log.Printf("alert evaluator: %v", err)
			}
		}
	}
}

func (e *Evaluator) EvaluateOnce(ctx context.Context) error {
	rules, err := e.store.ListActiveAlertRules()
	if err != nil {
		return fmt.Errorf("list rules: %w", err)
	}

	now := time.Now()
	for _, rule := range rules {
		from := now.Add(-time.Duration(rule.DurationSec) * time.Second)
		pts, err := e.metrics.QueryMetrics(rule.AppID, metrics.TierRaw, from, now)
		if err != nil {
			log.Printf("evaluator: query metrics rule %d: %v", rule.ID, err)
			continue
		}
		if len(pts) == 0 {
			continue
		}

		allSatisfy := true
		var sum float64
		for _, pt := range pts {
			v := extractMetricValue(pt, rule.Metric)
			sum += v
			if !checkCondition(v, rule.Operator, rule.Threshold) {
				allSatisfy = false
			}
		}
		avg := sum / float64(len(pts))

		active, err := e.store.GetActiveAlert(rule.ID)
		if err != nil {
			log.Printf("evaluator: get active alert rule %d: %v", rule.ID, err)
			continue
		}

		if allSatisfy && active == nil {
			// fire
			wh, err := e.store.GetWebhook(rule.WebhookID)
			if err != nil {
				log.Printf("evaluator: get webhook rule %d: %v", rule.ID, err)
				continue
			}
			event := e.buildEvent(rule, avg, "firing", now)
			if err := e.dispatcher.Send(*wh, event); err != nil {
				log.Printf("evaluator: send webhook rule %d: %v", rule.ID, err)
			}
			if _, err := e.store.CreateAlertHistory(rule.ID, avg); err != nil {
				log.Printf("evaluator: create history rule %d: %v", rule.ID, err)
			}
		} else if !allSatisfy && active != nil {
			// resolve
			if err := e.store.ResolveAlert(active.ID); err != nil {
				log.Printf("evaluator: resolve alert rule %d: %v", rule.ID, err)
				continue
			}
			wh, err := e.store.GetWebhook(rule.WebhookID)
			if err != nil {
				log.Printf("evaluator: get webhook rule %d: %v", rule.ID, err)
				continue
			}
			event := e.buildEvent(rule, avg, "resolved", now)
			if err := e.dispatcher.Send(*wh, event); err != nil {
				log.Printf("evaluator: send resolved webhook rule %d: %v", rule.ID, err)
			}
		}
	}
	return nil
}

func (e *Evaluator) buildEvent(rule store.AlertRule, value float64, status string, firedAt time.Time) AlertEvent {
	event := AlertEvent{
		Metric:    rule.Metric,
		Value:     value,
		Threshold: rule.Threshold,
		Operator:  rule.Operator,
		Status:    status,
		FiredAt:   firedAt,
	}
	if rule.AppID != nil {
		app, err := e.appLookup.GetAppByID(*rule.AppID)
		if err == nil {
			event.AppName = app.Name
			event.AppSlug = app.Slug
		}
	}
	return event
}

func checkCondition(value float64, operator string, threshold float64) bool {
	switch operator {
	case ">":
		return value > threshold
	case "<":
		return value < threshold
	case ">=":
		return value >= threshold
	case "<=":
		return value <= threshold
	}
	return false
}

func extractMetricValue(point metrics.MetricPoint, metric string) float64 {
	switch metric {
	case "cpu_pct":
		return point.CPUPct
	case "mem_bytes":
		return float64(point.MemBytes)
	case "mem_pct":
		if point.MemLimit == 0 {
			return 0
		}
		return float64(point.MemBytes) / float64(point.MemLimit) * 100
	}
	return 0
}
