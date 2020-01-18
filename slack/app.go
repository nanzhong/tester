package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"

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

	// log.Printf("command: %s", cmd.Command)
	// log.Printf("text: %s", cmd.Text)

	if s.scheduler == nil {
		message := &slack.Msg{
			ResponseType: "in_channel",
			Text:         "Slack integration not configured for scheduling tests.",
		}

		json.NewEncoder(w).Encode(message)
		return
	}

	packageRE := regexp.MustCompile(`^([[:alpha:]][[:alnum:]]+)\s*`)
	// TODO need to make this actually handle things like escaped characters,
	// quoted values, etc.
	// optionRE := regexp.MustCompile(`-?-([[:alnum:]]+)=?([^\s+]*)?`)
	packageMatches := packageRE.FindStringSubmatch(cmd.Text)
	// log.Printf("package match: %#v", packageMatches)
	// optionMatches := optionRE.FindAllStringSubmatch(cmd.Text, -1)
	// log.Printf("options match: %#v", optionMatches)

	if len(packageMatches) < 2 {
		message := &slack.Msg{
			ResponseType: "in_channel",
			Text:         fmt.Sprintf("Unable to parse %s %s", cmd.Command, cmd.Text),
		}

		json.NewEncoder(w).Encode(message)
		return
	}
	packageName := packageMatches[1]

	run, err := s.scheduler.Schedule(r.Context(), packageName)
	if err != nil {
		message := &slack.Msg{
			ResponseType: "in_channel",
			Text:         fmt.Sprintf("Failed to schedule test run for package %s: %s", packageName, err),
		}

		json.NewEncoder(w).Encode(message)
		return
	}
	runURL := fmt.Sprintf("%s/runs/%s", s.baseURL, run.ID)

	messageText := slack.NewTextBlockObject(slack.MarkdownType, "Started new test run", false, false)
	messageSection := slack.NewSectionBlock(messageText, nil, nil)

	runHeaderText := slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*<%s|Test Run %s>*", runURL, run.ID), false, false)
	runFields := []*slack.TextBlockObject{
		slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Package:* %s", packageName), false, false),
	}
	runSection := slack.NewSectionBlock(runHeaderText, runFields, nil)

	// if len(optionMatches) > 0 {
	// 	optionsHeaderField := slack.NewTextBlockObject(slack.MarkdownType, "*Options:*", false, false)
	// 	fields = append(fields, optionsHeaderField)
	// }

	// for _, option := range optionMatches {
	// 	var (
	// 		optionName  string
	// 		optionValue string
	// 	)
	// 	switch len(option) {
	// 	case 1:
	// 		continue
	// 	case 2:
	// 		optionName = option[1]
	// 		optionValue = "True"
	// 	case 3:
	// 		optionName = option[1]
	// 		optionValue = option[2]
	// 	default:
	// 		continue
	// 	}

	// 	optionField := slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("\t*%s:* %s", optionName, optionValue), false, false)
	// 	fields = append(fields, optionField)
	// }

	// statusText := slack.NewTextBlockObject(slack.MarkdownType, "*Pending*", false, false)
	// statusSection := slack.NewContextBlock("", statusText)

	message := slack.NewBlockMessage(
		messageSection,
		slack.NewDividerBlock(),
		runSection,
		// slack.NewDividerBlock(),
		// statusSection,
	)
	message.ResponseType = slack.ResponseTypeInChannel

	b, err := json.MarshalIndent(message, "", "    ")
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(string(b))

	json.NewEncoder(w).Encode(message)
}

func (s *App) Fire(ctx context.Context, alert *alerting.Alert) error {
	testLink := fmt.Sprintf("%s/tests/%s", alert.BaseURL, alert.Test.ID)

	messageText := slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf(":red_circle: *FAIL* - %s - %s", alert.Test.Name, testLink), false, false)
	messageSection := slack.NewSectionBlock(messageText, nil, nil)
	slack.NewDividerBlock()

	testDetail := slack.Attachment{
		Color:     "#ff005f",
		Fallback:  fmt.Sprintf("%s with ID %s failed (%s).\n%s", alert.Test.Name, alert.Test.ID, alert.Test.Duration().String(), testLink),
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
