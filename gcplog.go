package gcplog

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"cloud.google.com/go/logging"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
)

// Logger is a standard logging interface.
// *log.Logger implements it.
type Logger interface {
	Print(...interface{})
	Printf(string, ...interface{})
	Println(...interface{})

	Fatal(...interface{})
	Fatalf(string, ...interface{})
	Fatalln(...interface{})

	Panic(...interface{})
	Panicf(string, ...interface{})
	Panicln(...interface{})
}

type ExtendedLogger interface {
	Logger

	WithRequest(*logging.HTTPRequest) ExtendedLogger
	With(labels map[string]string) ExtendedLogger

	Log(s Severity, msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
	Crit(msg string, args ...interface{})
}

// Stackdriver logs to GCP Stackdriver and also prints them to stdout.
type Stackdriver struct {
	gcpLogger *logging.Logger
	*log.Logger

	commonLabels map[string]string
	labels       map[string]string

	req *logging.HTTPRequest
}

func (s *Stackdriver) WithRequest(req *logging.HTTPRequest) ExtendedLogger {
	return &Stackdriver{
		gcpLogger:    s.gcpLogger,
		Logger:       s.Logger,
		commonLabels: s.commonLabels,
		labels:       s.labels,
		req:          req,
	}
}

func (s *Stackdriver) With(labels map[string]string) ExtendedLogger {
	l := s.labels
	if l == nil {
		l = Labels{}
	}
	for k, v := range labels {
		l[k] = v
	}
	return &Stackdriver{
		gcpLogger:    s.gcpLogger,
		Logger:       s.Logger,
		commonLabels: s.commonLabels,
		labels:       l,
		req:          s.req,
	}
}

type Severity = logging.Severity

type Labels = map[string]string

// getGCPProjectID returns GCP project id.
func getGCPProjectID() (string, error) {
	filename := os.Getenv(EnvConfig)
	if filename == "" {
		return "", fmt.Errorf("env var %s is not set", EnvConfig)
	}
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("read %s failed: %s", filename, err)
	}
	payload := struct {
		ProjectID string `json:"project_id"`
	}{}
	if err := json.Unmarshal(bytes, &payload); err != nil {
		return "", fmt.Errorf("unmarshal file %s failed: %s", filename, err)
	}
	return payload.ProjectID, nil
}

const appName = "nyancat"

// EnvConfig is the name of env variable pointing to
// json file with GCP credentials.
const EnvConfig = "GOOGLE_APPLICATION_CREDENTIALS"

func buildGCPLogger(cl map[string]string) *logging.Logger {
	projectID, err := getGCPProjectID()
	if err != nil {
		log.Printf("Failed to get GCP credentials: %s", err)
		return nil
	}
	client, err := logging.NewClient(context.Background(), projectID)
	if err != nil {
		log.Printf("Failed to create GCP logging client: %s", err)
		return nil
	}
	return client.Logger(
		appName,
		logging.CommonResource(&mrpb.MonitoredResource{
			Type:   "project",
			Labels: map[string]string{"project_id": projectID},
		}),
		logging.CommonLabels(cl),
	)
}

func New(cl map[string]string) *Stackdriver {
	sd := &Stackdriver{
		gcpLogger:    buildGCPLogger(cl),
		commonLabels: cl,
		Logger:       log.New(os.Stderr, "", log.LstdFlags),
	}
	if cl != nil {
		app := cl["app"]
		module := cl["module"]
		sd.Logger.SetPrefix(strings.TrimSpace(fmt.Sprintf("%s %s", app, module)) + " ")
	}
	return sd
}

func formatPayload(msg string, args ...interface{}) map[string]interface{} {
	result := map[string]interface{}{"message": msg}

	isKey := true
	var k string
	for i := range args {
		a := args[i]
		if isKey {
			k = a.(string)
			isKey = false
		} else {
			result[k] = a
			isKey = true
		}
	}
	return result
}

func (s *Stackdriver) Print(args ...interface{})   { s.Printf(fmt.Sprint(args...)) }
func (s *Stackdriver) Println(args ...interface{}) { s.Printf(fmt.Sprintln(args...)) }

func (s *Stackdriver) Printf(msg string, args ...interface{}) {
	s.log(logging.Info, msg, args...)
}

func (s *Stackdriver) log(sev Severity, msg string, args ...interface{}) {
	s.Logger.Printf(msg, args...)
	if s.gcpLogger != nil {
		s.gcpLogger.Log(logging.Entry{
			Severity:    sev,
			Payload:     fmt.Sprintf(msg, args...),
			Labels:      s.labels,
			HTTPRequest: s.req,
		})
	}
}

// Log is doing structural logging with provided severity.
func (s *Stackdriver) Log(sev Severity, msg string, args ...interface{}) {
	payload := formatPayload(msg, args...)
	b, err := json.Marshal(payload)
	if err != nil {
		s.Error("failed to marshal", "err", err)
	} else {
		s.Logger.Print(string(b))
	}
	if s.gcpLogger != nil {
		s.gcpLogger.Log(logging.Entry{
			Severity:    sev,
			Payload:     payload,
			Labels:      s.labels,
			HTTPRequest: s.req,
		})
	}
}

func (s *Stackdriver) Fatal(args ...interface{})   { s.Fatalf(fmt.Sprint(args...)) }
func (s *Stackdriver) Fatalln(args ...interface{}) { s.Fatalf(fmt.Sprintln(args...)) }

func (s *Stackdriver) Fatalf(msg string, args ...interface{}) {
	s.log(logging.Critical, msg, args...)
	if s.gcpLogger != nil {
		s.gcpLogger.Flush()
	}
	os.Exit(1)
}

func (s *Stackdriver) Panic(args ...interface{})   { s.Panicf(fmt.Sprint(args...)) }
func (s *Stackdriver) Panicln(args ...interface{}) { s.Panicf(fmt.Sprintln(args...)) }

func (s *Stackdriver) Panicf(msg string, args ...interface{}) {
	s.log(logging.Critical, msg, args...)
	if s.gcpLogger != nil {
		s.gcpLogger.Flush()
	}
	panic(fmt.Sprintf(msg, args...))
}

// Debug sends debug log message.
func (s *Stackdriver) Debug(msg string, args ...interface{}) {
	s.Log(logging.Debug, msg, args...)
}

// Info sends info log message.
func (s *Stackdriver) Info(msg string, args ...interface{}) {
	s.Log(logging.Info, msg, args...)
}

// Warn sends warn log message.
func (s *Stackdriver) Warn(msg string, args ...interface{}) {
	s.Log(logging.Warning, msg, args...)
}

// Error sends error log message.
func (s *Stackdriver) Error(msg string, args ...interface{}) {
	s.Log(logging.Error, msg, args...)
}

// Crit sends critical log message followed by os.Exit(1).
func (s *Stackdriver) Crit(msg string, args ...interface{}) {
	s.Log(logging.Critical, msg, args...)
	s.gcpLogger.Flush()
	os.Exit(1)
}

func (s *Stackdriver) Flush() error {
	if s.gcpLogger != nil {
		return s.gcpLogger.Flush()
	}
	return nil
}
