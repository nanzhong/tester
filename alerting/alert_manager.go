package alerting

import (
	"context"
	"fmt"

	"github.com/nanzhong/tester"
	"golang.org/x/sync/errgroup"
)

type Alert struct {
	Test *tester.Test

	BaseURL string
}

type Alerter interface {
	Fire(context.Context, *Alert) error
}

type AlertManager struct {
	baseURL  string
	alerters []Alerter
}

func NewAlertManager(baseURL string, alerters []Alerter) *AlertManager {
	return &AlertManager{
		baseURL:  baseURL,
		alerters: alerters,
	}
}

func (a *AlertManager) RegisterAlerter(alerter Alerter) {
	a.alerters = append(a.alerters, alerter)
}

func (a *AlertManager) Fire(ctx context.Context, alert *Alert) error {
	alert.BaseURL = a.baseURL

	var eg errgroup.Group
	for _, alerter := range a.alerters {
		alerter := alerter
		eg.Go(func() error {
			return alerter.Fire(ctx, alert)
		})
	}
	err := eg.Wait()
	if err != nil {
		return fmt.Errorf("firing alerts: %w", err)
	}
	return nil
}
