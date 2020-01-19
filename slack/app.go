package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/nanzhong/tester/alerting"
	"github.com/nanzhong/tester/scheduler"
	"github.com/nlopes/slack"
)

type options struct {
	username      string
	webhookURL    string
	signingSecret string

	baseURL   string
	scheduler *scheduler.Scheduler
}

type Option func(*options)

func WithBaseURL(url string) Option {
	return func(opts *options) {
		opts.baseURL = url
	}
}

func WithUsername(username string) Option {
	return func(opts *options) {
		opts.username = username
	}
}

func WithWebhookURL(webhookURL string) Option {
	return func(opts *options) {
		opts.webhookURL = webhookURL
	}
}

func WithSigningSecret(signingSecret string) Option {
	return func(opts *options) {
		opts.signingSecret = signingSecret
	}
}

func WithScheduler(scheduler *scheduler.Scheduler) Option {
	return func(opts *options) {
		opts.scheduler = scheduler
	}
}

type App struct {
	username      string
	webhookURL    string
	signingSecret string

	baseURL   string
	scheduler *scheduler.Scheduler

	usageMessage *slack.Message
}

func NewApp(opts ...Option) *App {
	defOpts := &options{
		username: "tester",
	}

	for _, opt := range opts {
		opt(defOpts)
	}

	return &App{
		username:      defOpts.username,
		webhookURL:    defOpts.webhookURL,
		signingSecret: defOpts.signingSecret,

		baseURL:   defOpts.baseURL,
		scheduler: defOpts.scheduler,
	}
}

func (s *App) HandleSlackCommand(w http.ResponseWriter, r *http.Request) {
	verifier, err := slack.NewSecretsVerifier(r.Header, s.signingSecret)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	r.Body = ioutil.NopCloser(io.TeeReader(r.Body, &verifier))
	cmd, err := slack.SlashCommandParse(r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err = verifier.Ensure(); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if s.scheduler == nil {
		message := &slack.Msg{
			Text: ":warning: Slack integration not configured for scheduling tests.",
		}

		json.NewEncoder(w).Encode(message)
		return
	}

	args := strings.Fields(cmd.Text)
	if len(args) < 1 {
		message := &slack.Msg{
			Text: fmt.Sprintf(":warning: Missing action. See `%s help`.", cmd.Command),
		}

		json.NewEncoder(w).Encode(message)
		return
	}

	switch strings.ToLower(args[0]) {
	case "help":
		json.NewEncoder(w).Encode(s.helpMessage(cmd.Command))
		return
	case "test":
		// continue through to handling the action.
	default:
		message := &slack.Msg{
			Text: fmt.Sprintf(":warning: Unknown action *%s*. See `%s help`.", args[0], cmd.Command),
		}

		json.NewEncoder(w).Encode(message)
		return
	}

	packageName := args[1]
	args = args[2:]
	run, err := s.scheduler.Schedule(r.Context(), packageName, args...)
	if err != nil {
		message := &slack.Msg{
			Text: fmt.Sprintf(":warning: Failed to schedule test run for package %s: *%s*", packageName, err),
		}

		json.NewEncoder(w).Encode(message)
		return
	}
	runURL := fmt.Sprintf("%s/runs/%s", s.baseURL, run.ID)

	messageText := slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf(":traffic_light:  *NEW* - Started new test run for package %s\n%s", packageName, runURL), false, false)
	messageSection := slack.NewSectionBlock(messageText, nil, nil)

	var options []string
	for _, option := range run.Package.Options {
		options = append(options, fmt.Sprintf("`%s`", option.String()))
	}
	runDetail := slack.Attachment{
		Color:     "#80cee1",
		Title:     run.Package.Name,
		TitleLink: runURL,
		Fields: []slack.AttachmentField{
			{
				Title: "Run ID",
				Value: run.ID,
			},
			{
				Title: "Options",
				Value: strings.Join(options, "\n"),
			},
		},

		Footer:     "tester",
		FooterIcon: "",
		Ts:         json.Number(strconv.FormatInt(run.EnqueuedAt.Unix(), 10)),
	}

	message := slack.NewBlockMessage(
		messageSection,
	)
	message.ResponseType = slack.ResponseTypeInChannel
	message.Attachments = append(message.Attachments, runDetail)

	json.NewEncoder(w).Encode(message)
}

func (s *App) Fire(ctx context.Context, alert *alerting.Alert) error {
	testLink := fmt.Sprintf("%s/tests/%s", alert.BaseURL, alert.Test.ID)

	messageText := slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf(":warning: *FAIL* - %s\n%s", alert.Test.Name, testLink), false, false)
	messageSection := slack.NewSectionBlock(messageText, nil, nil)

	testDetail := slack.Attachment{
		Color:     "#ff005f",
		Title:     alert.Test.Name,
		TitleLink: testLink,
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
		Ts:         json.Number(strconv.FormatInt(alert.Test.FinishedAt.Unix(), 10)),
	}

	err := slack.PostWebhook(s.webhookURL, &slack.WebhookMessage{
		Username: s.username,
		Blocks: []slack.Block{
			messageSection,
		},
		Attachments: []slack.Attachment{
			testDetail,
		},
	})
	if err != nil {
		return fmt.Errorf("firing slack alert: %w", err)
	}
	return nil
}

func (a *App) helpMessage(command string) *slack.Message {
	if a.usageMessage != nil {
		return a.usageMessage
	}

	// for readability here
	lines := []string{
		"```",
		"Trigger tests from Slack",
		"",
		fmt.Sprintf("Usage: %s <action> [arguments]", command),
		"",
		"The commands are:",
		"",
		"  help                      print this help message",
		"  test <package> [options]  trigger an e2e test",
		"",
		"Test packages:",
	}
	for _, pkg := range a.scheduler.Packages {
		lines = append(lines, "", fmt.Sprintf("  %s", pkg.Name))
		for _, option := range pkg.Options {
			description := fmt.Sprintf("      %s", option.Description)
			if option.Default != "" {
				description = description + fmt.Sprintf(" (default: %s)", option.Default)
			}
			lines = append(lines, fmt.Sprintf("    -%s", option.Name), description)
		}
	}
	lines = append(lines, "```")

	message := slack.NewBlockMessage(slack.NewSectionBlock(slack.NewTextBlockObject(slack.MarkdownType, strings.Join(lines, "\n"), false, false), nil, nil))
	a.usageMessage = &message
	return a.usageMessage
}
