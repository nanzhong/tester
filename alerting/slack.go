package alerting

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/nlopes/slack"
)

type slackOptions struct {
	username string
}

type SlackOption func(*slackOptions)

func WithSlackUsername(username string) SlackOption {
	return func(opts *slackOptions) {
		opts.username = username
	}
}

type SlackAlerter struct {
	username   string
	webhookURL string
}

func NewSlackAlerter(webhookURL string, opts ...SlackOption) *SlackAlerter {
	defOpts := &slackOptions{
		username: "tester",
	}

	for _, opt := range opts {
		opt(defOpts)
	}

	return &SlackAlerter{
		username:   defOpts.username,
		webhookURL: webhookURL,
	}
}

func (a *SlackAlerter) Fire(ctx context.Context, alert *Alert) error {
	testLink := fmt.Sprintf("%s/tests/%s", alert.baseURL, alert.Test.ID)
	err := slack.PostWebhook(a.webhookURL, &slack.WebhookMessage{
		Username: a.username,
		Attachments: []slack.Attachment{
			{
				Color:     "#ff005f",
				Fallback:  fmt.Sprintf("%s with ID %s failed (%s).\n%s", alert.Test.Name, alert.Test.ID, alert.Test.Duration().String(), testLink),
				Title:     alert.Test.Name,
				TitleLink: testLink,
				Text:      "Failure running test",
				Fields: []slack.AttachmentField{
					{
						Title: "Test ID",
						Value: alert.Test.ID,
						Short: true,
					},
					{
						Title: "Duration",
						Value: alert.Test.Duration().String(),
						Short: true,
					},
				},

				Footer:     "tester",
				FooterIcon: "",
				Ts:         json.Number(strconv.FormatInt(alert.Test.FinishTime.Unix(), 10)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("firing slack alert: %w", err)
	}
	return nil
}
